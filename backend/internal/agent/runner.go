package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// TaskRunner executes tasks headlessly: when the scheduler fires (trigger
// time reached + dependencies met), the runner dials the 1acp bridge-server
// directly — no frontend involved — injects the issue background, sends the
// task description as the work instruction, and writes the agent's replies
// back to the timeline. The web UI is an observation surface, not part of
// the execution path (project-model: 自动化定时看板).
type TaskRunner struct {
	serverPort int
	// selfBaseURL is the daemon's loopback HTTP base (e.g. http://127.0.0.1:PORT)
	// — the verifier's tasks MCP subprocess calls back into it via submit_review.
	// Distinct from serverPort, which is the 1acp bridge (WebSocket) port.
	selfBaseURL string
	tasksStore  *TasksStore
	chatStore   *Store
	scheduler   *Scheduler
}

// NewTaskRunner wires a runner over the same stores the HTTP handlers use.
func NewTaskRunner(serverPort int, selfBaseURL string, tasksStore *TasksStore, chatStore *Store, scheduler *Scheduler) *TaskRunner {
	return &TaskRunner{
		serverPort:  serverPort,
		selfBaseURL: selfBaseURL,
		tasksStore:  tasksStore,
		chatStore:   chatStore,
		scheduler:   scheduler,
	}
}

// idleTimeout aborts a run when the bridge goes silent (hung agent would
// otherwise hold the workspace lock forever).
const runnerIdleTimeout = 10 * time.Minute

// Execute runs one task to completion. Blocking — the scheduler invokes it
// in a goroutine. The caller must already hold the workspace lock and have
// marked the task running; Execute releases the lock and persists the
// terminal status on exit.
func (r *TaskRunner) Execute(workspacePath, workspaceID string, task Task) {
	// Release the workspace lock, then immediately re-tick so any task that
	// was blocked on this one advances at once instead of waiting up to 5s
	// for the next scheduler tick (即时接力).
	defer func() {
		r.scheduler.Lock.Release(workspacePath)
		r.scheduler.Tick()
	}()

	// Card content is YAML-frontmatter Markdown: execute against the prose body,
	// and treat acceptance from the frontmatter (or the legacy column) as the
	// self-check gate.
	instruction := buildTaskInstruction(task)
	if instruction == "" {
		r.finish(workspacePath, task.ID, "", TaskStatusFailed, "task has no description/title to execute")
		return
	}

	agentType := task.Assignee
	if agentType == "" {
		agentType = DefaultAgentType
	}
	idleTimeout := runnerIdleTimeout
	if task.TimeoutMinutes > 0 {
		idleTimeout = time.Duration(task.TimeoutMinutes) * time.Minute
	}

	// Index a chat session record so the run shows up in the sidebar (with
	// the task badge) and the transcript is reachable afterwards.
	sessionID := newID()
	rec := ChatSessionRecord{
		ID:          sessionID,
		WorkspaceID: workspaceID,
		TaskID:      task.ID,
		Name:        fmt.Sprintf("%s - 自动执行", task.Title),
		AgentType:   agentType,
		// Headless auto-runs are backend-silent: Role "auto" keeps this record
		// out of the sidebar session list (see handler.list) so an AI-executed
		// task doesn't spawn a chat box. The record still exists so "查看详情"
		// can resume the transcript by id afterwards.
		Role: SessionRoleAuto,
		// Unattended runs must not block on permission prompts: nobody is
		// at the browser to approve, so a pending request would time out
		// and fail the task (confirmed decision: approve-all).
		PermissionMode: "approve-all",
	}
	if err := r.chatStore.Add(rec); err != nil {
		log.Printf("[runner] index session for task %s: %v", task.ID, err)
	}
	r.attachSessionMetadata(workspacePath, task.ID, sessionID, agentType)

	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", r.serverPort)
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		r.finish(workspacePath, task.ID, sessionID, TaskStatusFailed, "bridge unavailable: "+err.Error())
		return
	}
	defer conn.Close()

	ensure := WsMessage{
		Action:         "ensure_session",
		SessionID:      sessionID,
		WorkspacePath:  workspacePath,
		AgentType:      agentType,
		SystemContext:  buildIssueBackground(&task, workspacePath),
		PermissionMode: "approve-all",
	}
	if err := conn.WriteJSON(ensure); err != nil {
		r.finish(workspacePath, task.ID, sessionID, TaskStatusFailed, "ensure_session failed: "+err.Error())
		return
	}

	// Reuse the bridged-path accumulator so the timeline write-back has the
	// exact same semantics (output deltas, reset on tool_call, flush on done).
	bridge := &ActiveBridge{
		SessionID:     sessionID,
		WorkspacePath: workspacePath,
		TaskID:        task.ID,
		AgentType:     agentType,
	}

	log.Printf("[runner] Auto-executing task %s (%q) in %s, session %s", task.ID, task.Title, workspacePath, sessionID)

	promptSent := false
	for {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		var msg WsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			r.finish(workspacePath, task.ID, sessionID, TaskStatusFailed, "bridge read failed: "+err.Error())
			return
		}

		switch msg.Event {
		case "session_ready":
			if msg.AgentSessionID != "" {
				_ = r.chatStore.UpdateACP(sessionID, msg.AgentSessionID)
			}
			if !promptSent {
				promptSent = true
				if err := conn.WriteJSON(WsMessage{
					Action:    "prompt",
					SessionID: sessionID,
					Text:      instruction,
				}); err != nil {
					r.finish(workspacePath, task.ID, sessionID, TaskStatusFailed, "prompt failed: "+err.Error())
					return
				}
			}
		case "text_delta":
			if msg.Type != "thought" && msg.Text != "" {
				bridge.appendTurnText(msg.Text)
			}
		case "tool_call":
			bridge.resetTurnText()
		case "done":
			writeAgentReply(bridge, r.tasksStore, r.chatStore)
			summary := msg.Summary
			if summary == "" {
				summary = "Execution completed."
			}
			// #50: a task configured for verification doesn't complete here — it
			// hands off to pending_review, where the scheduler runs a headless
			// verifier pass (r.Verify). Without a verifier it completes as before.
			terminal := TaskStatusCompleted
			if needsReview(&task) {
				terminal = TaskStatusPendingReview
			}
			r.finish(workspacePath, task.ID, sessionID, terminal, summary)
			// Politely close the agent session so the runtime doesn't keep
			// an idle process around for a finished scheduled task.
			_ = conn.WriteJSON(WsMessage{Action: "close_session", SessionID: sessionID})
			return
		case "error":
			r.finish(workspacePath, task.ID, sessionID, TaskStatusFailed, "agent error: "+msg.Message)
			return
		}
	}
}

func buildTaskInstruction(task Task) string {
	_, instruction := SplitFrontmatter(task.Description)
	if instruction == "" {
		instruction = task.Title
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return ""
	}

	acceptance := task.AcceptanceCriteria
	if fm := FrontmatterAcceptance(task.Description); fm != "" {
		acceptance = fm
	}
	if acceptance != "" {
		instruction += "\n\n完成后请对照验收标准自查；若未达标，请明确说明原因。\n\n=== 验收标准 ===\n" + acceptance
	}
	return instruction + "\n\n" + projectExecutorPrompt
}

const projectExecutorPrompt = `=== project_executor ===
你是当前项目里被调度执行这一条任务的执行者。请专注完成任务正文和验收标准要求的交付物,并通过 Bash 调用内置 CLI 了解和更新任务状态。

## 任务看板 CLI
- 开始前用 1agents project-items get <任务ID> 查看当前任务详情、description、acceptanceCriteria、dependsOn 和 timeline。
- 需要确认上下文或依赖时,用 1agents project-items list、1agents project-items graph <任务ID> 阅读同项目条目和引用关系。
- 执行过程中如需记录进展、卡点或取消,用 1agents project-items update <任务ID> ... 更新当前任务。不要创建新任务、不要修改其他任务、不要调整里程碑;这些属于 PM。
- 完成后不要依赖口头自报。先运行必要检查,再在最终回复里简明说明改了什么、验证结果、是否仍有未达标项。系统会根据执行结果和核验流程推进任务状态。

## 执行原则
- 先读任务,再动手;以 acceptanceCriteria 为准逐项自查。
- 只做当前任务要求的最小改动,不顺手重构无关代码。
- 遇到缺信息、依赖未完成或需要人做产品/架构取舍时,明确写出阻塞原因和需要谁决定什么,不要编造。`

// Verify runs a headless verification cycle over a task whose executor finished
// (status pending_review). Blocking — the scheduler invokes it in a goroutine
// after marking the task running and acquiring the workspace lock.
//
// Adversarial panel (#131): when the task configures VerifierCount > 1, Verify
// runs that many independent verifier sessions sequentially. Each verifier gets
// a hard read-only tasks MCP (no update_project_item; submit_review only) and submits
// its own verdict; the server (applyReviewVerdict) pools the verdicts and, once
// the panel is complete, aggregates them by threshold to drive the state machine
// (#50). A verifier that ends without submitting counts as a rejecting verdict
// so the panel can't stall. Releases the lock and re-ticks on exit.
func (r *TaskRunner) Verify(workspacePath, workspaceID string, task Task) {
	defer func() {
		r.scheduler.Lock.Release(workspacePath)
		r.scheduler.Tick()
	}()

	verifier := task.Verifier
	if verifier == "" {
		verifier = DefaultAgentType
	}

	// Defensive: a task should only reach Verify when configured for it.
	if !needsReview(&task) {
		r.finish(workspacePath, task.ID, "", TaskStatusCompleted, "无需核验,直接完成")
		return
	}

	n := effectiveVerifierCount(&task)
	log.Printf("[runner] Verifying task %s (%q) in %s with %d verifier(s), threshold %d",
		task.ID, task.Title, workspacePath, n, effectivePassThreshold(&task))

	for pass := 1; pass <= n; pass++ {
		// Stop early if a panelist's verdict already drove the task off running
		// (e.g. an aggregate decision after a no-verdict reject filled the pool).
		if cur, ok, _ := r.tasksStore.GetTask(task.ID); ok && cur.Status != TaskStatusRunning {
			return
		}
		r.runVerifierPass(workspacePath, workspaceID, task, verifier, pass, n)
	}
}

// runVerifierPass drives one independent verifier session: spin up a headless
// agent with the verifier persona + read-only tasks MCP, prompt it to judge,
// and wait for it to finish. The verdict reaches the task via submit_review →
// POST /review → applyReviewVerdict, which pools it. If the session ends without
// adding a verdict to the pool, a synthetic rejection is recorded so the panel
// advances. See Verify.
func (r *TaskRunner) runVerifierPass(workspacePath, workspaceID string, task Task, verifier string, pass, total int) {
	// Verifier persona: role template (verifier.md) with project context. The
	// hard read-only tool surface is enforced server-side regardless, so a
	// missing template only loses the persona, not the safety.
	persona := ""
	if tpl, ok := LoadRoles(workspacePath).Resolve(SessionRoleVerifier); ok {
		persona = renderRolePrompt(tpl, resolveWorkspaceName(workspaceID), workspaceID)
	}
	systemContext := buildIssueBackground(&task, workspacePath)
	if persona != "" {
		systemContext = persona + "\n\n" + systemContext
	}

	// Tasks MCP locked to this task in verifier scope (read-only + submit_review),
	// calling back into this daemon's HTTP API.
	var mcpServers json.RawMessage
	if srv := tasksMcpServerEntry(r.selfBaseURL, workspaceID, task.ID, SessionRoleVerifier); srv != nil {
		if b, err := json.Marshal([]map[string]any{srv}); err == nil {
			mcpServers = b
		}
	}

	idleTimeout := runnerIdleTimeout
	if task.TimeoutMinutes > 0 {
		idleTimeout = time.Duration(task.TimeoutMinutes) * time.Minute
	}

	// poolBefore lets us tell whether this pass actually added a verdict.
	poolBefore := 0
	if cur, ok, _ := r.tasksStore.GetTask(task.ID); ok {
		poolBefore = len(cur.ReviewPool)
	}

	// Headless verifier session, hidden from the sidebar like auto-runs.
	sessionID := newID()
	name := fmt.Sprintf("%s - 自动核验", task.Title)
	if total > 1 {
		name = fmt.Sprintf("%s - 对抗式核验 %d/%d", task.Title, pass, total)
	}
	rec := ChatSessionRecord{
		ID:             sessionID,
		WorkspaceID:    workspaceID,
		TaskID:         task.ID,
		Name:           name,
		AgentType:      verifier,
		Role:           SessionRoleAuto,
		PermissionMode: "approve-all",
	}
	if err := r.chatStore.Add(rec); err != nil {
		log.Printf("[runner] index verify session for task %s: %v", task.ID, err)
	}
	r.attachSessionMetadata(workspacePath, task.ID, sessionID, verifier)

	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", r.serverPort)
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		r.rejectNoVerdict(workspacePath, task.ID, verifier, poolBefore, "bridge unavailable: "+err.Error())
		return
	}
	defer conn.Close()

	ensure := WsMessage{
		Action:         "ensure_session",
		SessionID:      sessionID,
		WorkspacePath:  workspacePath,
		AgentType:      verifier,
		SystemContext:  systemContext,
		McpServers:     mcpServers,
		PermissionMode: "approve-all",
	}
	if err := conn.WriteJSON(ensure); err != nil {
		r.rejectNoVerdict(workspacePath, task.ID, verifier, poolBefore, "ensure_session failed: "+err.Error())
		return
	}

	instruction := "执行者已提交产出。请独立、默认怀疑地逐条对照本任务的验收标准核验执行者的产出,主动找漏洞与反例,然后调用 submit_review 提交裁决:每条标准报告 pass(是否达标),未达标的写明缺什么。只有全部达标才算这条通过,否则会被打回重做。若问题不是执行者重做能解决的,而是需要人来做设计/架构/取舍决策,则改为在裁决中设 needsHuman=true,并在 summary 写清需要人决定什么——系统会升级等待人工,而不是空转重做。"

	log.Printf("[runner] Verify pass %d/%d task %s, session %s", pass, total, task.ID, sessionID)

	promptSent := false
	for {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		var msg WsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			r.rejectNoVerdict(workspacePath, task.ID, verifier, poolBefore, "bridge read failed: "+err.Error())
			return
		}
		switch msg.Event {
		case "session_ready":
			if msg.AgentSessionID != "" {
				_ = r.chatStore.UpdateACP(sessionID, msg.AgentSessionID)
			}
			if !promptSent {
				promptSent = true
				if err := conn.WriteJSON(WsMessage{Action: "prompt", SessionID: sessionID, Text: instruction}); err != nil {
					r.rejectNoVerdict(workspacePath, task.ID, verifier, poolBefore, "prompt failed: "+err.Error())
					return
				}
			}
		case "done":
			// The verdict (via submit_review → applyReviewVerdict) was pooled. If
			// this pass added nothing to the pool, the verifier ended without a
			// verdict — record a synthetic rejection so the panel advances.
			r.markVerifySessionIdle(workspacePath, task.ID, sessionID)
			r.rejectNoVerdict(workspacePath, task.ID, verifier, poolBefore, "核验者未提交裁决")
			_ = conn.WriteJSON(WsMessage{Action: "close_session", SessionID: sessionID})
			return
		case "error":
			r.rejectNoVerdict(workspacePath, task.ID, verifier, poolBefore, "verifier agent error: "+msg.Message)
			return
		}
	}
}

// rejectNoVerdict records a synthetic rejecting verdict when a verification pass
// ends without the verifier submitting one (crash, hang, or a verifier that
// simply forgot the tool). It reuses applyReviewVerdict so the same pool/budget/
// loop rules apply. poolBefore is the pool length when the pass started: if the
// pool grew, this pass already submitted and we no-op. Also no-ops if the task
// already left running (the panel's aggregate decision landed).
func (r *TaskRunner) rejectNoVerdict(workspacePath, taskID, verifier string, poolBefore int, reason string) {
	cur, ok, _ := r.tasksStore.GetTask(taskID)
	if !ok || cur.Status != TaskStatusRunning {
		return
	}
	if len(cur.ReviewPool) > poolBefore {
		return // this pass already submitted a verdict
	}
	crit := []CriterionResult{{Criterion: "核验未完成", Pass: false, Comment: reason}}
	if _, err := applyReviewVerdict(r.tasksStore, workspacePath, taskID, crit, false, reason, verifier); err != nil {
		log.Printf("[runner] verify task %s: record no-verdict rejection: %v", taskID, err)
	}
}

// markVerifySessionIdle flips the verify session's metadata to idle (the
// verdict itself owns the task status transition).
func (r *TaskRunner) markVerifySessionIdle(workspacePath, taskID, sessionID string) {
	_ = r.tasksStore.Mutate(workspacePath, func(cfg *TasksConfig) bool {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID != taskID {
				continue
			}
			for j := range cfg.Tasks[i].Sessions {
				if cfg.Tasks[i].Sessions[j].ID == sessionID {
					cfg.Tasks[i].Sessions[j].Status = SessionStatusIdle
					return true
				}
			}
		}
		return false
	})
}

// attachSessionMetadata records the run on Task.Sessions (status running).
func (r *TaskRunner) attachSessionMetadata(workspacePath, taskID, sessionID, agentType string) {
	_ = r.tasksStore.Mutate(workspacePath, func(cfg *TasksConfig) bool {
		for i := range cfg.Tasks {
			task := &cfg.Tasks[i]
			if task.ID != taskID {
				continue
			}
			task.Sessions = append(task.Sessions, SessionMetadata{
				ID:        sessionID,
				Kind:      SessionKindChat,
				Name:      "自动执行",
				AgentType: agentType,
				Status:    SessionStatusRunning,
				CreatedAt: time.Now().UTC(),
			})
			return true
		}
		return false
	})
}

// finish persists the terminal state of an automated run.
func (r *TaskRunner) finish(workspacePath, taskID, sessionID string, status TaskStatus, summary string) {
	now := time.Now().UTC()
	var title string
	var number int
	err := r.tasksStore.Mutate(workspacePath, func(cfg *TasksConfig) bool {
		for i := range cfg.Tasks {
			task := &cfg.Tasks[i]
			if task.ID != taskID {
				continue
			}
			task.Status = status
			task.Summary = summary
			task.UpdatedAt = now
			if status == TaskStatusCompleted {
				task.CompletedAt = &now
			}
			title, number = task.Title, task.Number
			for j := range task.Sessions {
				if task.Sessions[j].ID == sessionID {
					task.Sessions[j].Status = SessionStatusIdle
					task.Sessions[j].Summary = summary
				}
			}
			return true
		}
		return false
	})
	if err != nil {
		log.Printf("[runner] finish save %s: %v", taskID, err)
		return
	}
	log.Printf("[runner] Task %s finished: %s (%s)", taskID, status, summary)

	// failed / pending_review → push an IM approve/reject card (#129).
	var kind TaskNotifyKind
	switch status {
	case TaskStatusFailed:
		kind = NotifyFailed
	case TaskStatusPendingReview:
		kind = NotifyPendingReview
	default:
		return
	}
	emitNotify(TaskNotification{
		Kind:          kind,
		WorkspacePath: workspacePath,
		TaskID:        taskID,
		Number:        number,
		Title:         title,
		Summary:       summary,
	})
}
