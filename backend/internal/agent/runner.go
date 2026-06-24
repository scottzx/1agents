package agent

import (
	"encoding/json"
	"fmt"
	"log"
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
	_, instruction := SplitFrontmatter(task.Description)
	if instruction == "" {
		instruction = task.Title
	}
	if instruction == "" {
		r.finish(workspacePath, task.ID, "", TaskStatusFailed, "task has no description/title to execute")
		return
	}
	acceptance := task.AcceptanceCriteria
	if fm := FrontmatterAcceptance(task.Description); fm != "" {
		acceptance = fm
	}
	if acceptance != "" {
		instruction += "\n\n完成后请对照验收标准自查；若未达标，请明确说明原因。\n\n=== 验收标准 ===\n" + acceptance
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

// Verify runs a headless verification pass over a task whose executor finished
// (status pending_review). Blocking — the scheduler invokes it in a goroutine
// after marking the task running and acquiring the workspace lock. The verifier
// gets a hard read-only tasks MCP (no update_task; submit_review only) locked to
// this task; its verdict, submitted via submit_review → POST /review, drives the
// state machine (applyReviewVerdict). If the agent finishes without submitting a
// verdict, that is treated as a rejection so the loop can't stall. Releases the
// lock and re-ticks on exit. See #50.
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

	// Headless verifier session, hidden from the sidebar like auto-runs.
	sessionID := newID()
	rec := ChatSessionRecord{
		ID:             sessionID,
		WorkspaceID:    workspaceID,
		TaskID:         task.ID,
		Name:           fmt.Sprintf("%s - 自动核验", task.Title),
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
		r.rejectNoVerdict(workspacePath, task.ID, verifier, "bridge unavailable: "+err.Error())
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
		r.rejectNoVerdict(workspacePath, task.ID, verifier, "ensure_session failed: "+err.Error())
		return
	}

	instruction := "执行者已提交产出。请逐条对照本任务的验收标准核验执行者的产出,然后调用 submit_review 提交裁决:每条标准报告 pass(是否达标),未达标的写明缺什么。只有全部达标任务才算完成,否则会被打回重做。"

	log.Printf("[runner] Verifying task %s (%q) in %s, session %s", task.ID, task.Title, workspacePath, sessionID)

	promptSent := false
	for {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		var msg WsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			r.rejectNoVerdict(workspacePath, task.ID, verifier, "bridge read failed: "+err.Error())
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
					r.rejectNoVerdict(workspacePath, task.ID, verifier, "prompt failed: "+err.Error())
					return
				}
			}
		case "done":
			// The verdict (via submit_review → applyReviewVerdict) already moved
			// the task off running. If it didn't, the verifier ended without a
			// verdict — treat that as a rejection so the loop advances.
			r.markVerifySessionIdle(workspacePath, task.ID, sessionID)
			if cur, ok, _ := r.tasksStore.GetTask(task.ID); ok && cur.Status == TaskStatusRunning {
				r.rejectNoVerdict(workspacePath, task.ID, verifier, "核验者未提交裁决")
			}
			_ = conn.WriteJSON(WsMessage{Action: "close_session", SessionID: sessionID})
			return
		case "error":
			r.rejectNoVerdict(workspacePath, task.ID, verifier, "verifier agent error: "+msg.Message)
			return
		}
	}
}

// rejectNoVerdict records a synthetic rejection when a verification pass ends
// without the verifier submitting a verdict (crash, hang, or a verifier that
// simply forgot the tool). It reuses applyReviewVerdict so the same budget/loop
// rules apply. No-op if the task already left running (a verdict landed).
func (r *TaskRunner) rejectNoVerdict(workspacePath, taskID, verifier, reason string) {
	if cur, ok, _ := r.tasksStore.GetTask(taskID); !ok || cur.Status != TaskStatusRunning {
		return
	}
	crit := []CriterionResult{{Criterion: "核验未完成", Pass: false, Comment: reason}}
	if _, err := applyReviewVerdict(r.tasksStore, workspacePath, taskID, crit, reason, verifier); err != nil {
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
}
