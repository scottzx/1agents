package agent

import (
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
	tasksStore *TasksStore
	chatStore  *Store
	scheduler  *Scheduler
}

// NewTaskRunner wires a runner over the same stores the HTTP handlers use.
func NewTaskRunner(serverPort int, tasksStore *TasksStore, chatStore *Store, scheduler *Scheduler) *TaskRunner {
	return &TaskRunner{
		serverPort: serverPort,
		tasksStore: tasksStore,
		chatStore:  chatStore,
		scheduler:  scheduler,
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

	instruction := task.Description
	if instruction == "" {
		instruction = task.Title
	}
	if instruction == "" {
		r.finish(workspacePath, task.ID, "", TaskStatusFailed, "task has no description/title to execute")
		return
	}
	if task.AcceptanceCriteria != "" {
		instruction += "\n\n完成后请对照验收标准自查；若未达标，请明确说明原因。"
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
			r.finish(workspacePath, task.ID, sessionID, TaskStatusCompleted, summary)
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

// attachSessionMetadata records the run on Task.Sessions (status running).
func (r *TaskRunner) attachSessionMetadata(workspacePath, taskID, sessionID, agentType string) {
	cfg, err := r.tasksStore.Load(workspacePath)
	if err != nil {
		return
	}
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
		_ = r.tasksStore.Save(workspacePath, cfg)
		return
	}
}

// finish persists the terminal state of an automated run.
func (r *TaskRunner) finish(workspacePath, taskID, sessionID string, status TaskStatus, summary string) {
	cfg, err := r.tasksStore.Load(workspacePath)
	if err != nil {
		log.Printf("[runner] finish load %s: %v", taskID, err)
		return
	}
	now := time.Now().UTC()
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
		if err := r.tasksStore.Save(workspacePath, cfg); err != nil {
			log.Printf("[runner] finish save %s: %v", taskID, err)
		}
		log.Printf("[runner] Task %s finished: %s (%s)", taskID, status, summary)
		return
	}
}
