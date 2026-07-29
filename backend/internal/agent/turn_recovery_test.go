package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestRecoverInterruptedTurnsReconcilesDurableRuntimeTerminal(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}
	defer db.Close()
	if err := db.EnsureProject("project-1", "Turn recovery", t.TempDir()); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := meta.NewSessionStore(db).Add(meta.ChatSessionRecord{
		ID: "session-1", WorkspaceID: "project-1", AgentType: "codex",
	}); err != nil {
		t.Fatalf("Add session: %v", err)
	}

	store := meta.NewAgentTurnStore(db)
	running, _, err := store.Create(meta.AgentTurn{
		ID: "turn-running", ProjectID: "project-1", SessionID: "session-1",
		ClientRequestID: "request-running", PromptText: "finish this",
	})
	if err != nil {
		t.Fatalf("Create running: %v", err)
	}
	if _, err := store.Transition(running.ID, meta.AgentTurnTransition{
		Status: meta.AgentTurnRunning,
	}); err != nil {
		t.Fatalf("Start running: %v", err)
	}
	queued, _, err := store.Create(meta.AgentTurn{
		ID: "turn-queued", ProjectID: "project-1", SessionID: "session-1",
		ClientRequestID: "request-queued", PromptText: "do not replay",
	})
	if err != nil {
		t.Fatalf("Create queued: %v", err)
	}

	stateDir := t.TempDir()
	record := map[string]any{
		"messages": []any{
			map[string]any{"User": map[string]any{
				"id": "turn-running", "content": []any{map[string]any{"Text": "finish this"}},
			}},
			map[string]any{"Agent": map[string]any{
				"content":      []any{map[string]any{"Text": "durable final"}},
				"tool_results": map[string]any{},
			}},
		},
		"acpx": map[string]any{"turn_results": map[string]any{
			"turn-running": map[string]any{
				"status":            "completed",
				"prompt_message_id": "turn-running",
				"started_at":        "2026-07-29T01:00:00Z",
				"completed_at":      "2026-07-29T01:01:00Z",
				"stop_reason":       "end_turn",
				"last_event_seq":    9,
			},
		}},
	}
	raw, _ := json.Marshal(record)
	if err := os.WriteFile(
		filepath.Join(stateDir, "session-1.json"),
		raw,
		0o600,
	); err != nil {
		t.Fatalf("Write runtime record: %v", err)
	}

	failed, cancelled, reconciled, err := recoverInterruptedTurns(store, stateDir)
	if err != nil || failed != 0 || cancelled != 1 || reconciled != 1 {
		t.Fatalf("recover: failed=%d cancelled=%d reconciled=%d err=%v",
			failed, cancelled, reconciled, err)
	}
	recovered, _, _ := store.Get(running.ID)
	if recovered.Status != meta.AgentTurnCompleted ||
		recovered.FinalAnswer != "durable final" ||
		recovered.TerminalSource != "reconciled_runtime_record" ||
		recovered.PromptMessageID != running.ID ||
		recovered.LastEventSeq != 9 {
		t.Fatalf("reconciled Turn: %+v", recovered)
	}
	cancelledTurn, _, _ := store.Get(queued.ID)
	if cancelledTurn.Status != meta.AgentTurnCancelled ||
		cancelledTurn.TerminalSource != "recovery_policy" {
		t.Fatalf("queued recovery: %+v", cancelledTurn)
	}
}
