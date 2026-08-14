package agent

import (
	"encoding/json"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestIngestTurnChangeReportsSkipsCurrentRecipeUnlessJustFinished(t *testing.T) {
	h, db, _, _, turn := mutationAttributionRig(t)
	defer db.Close()

	client := NewAcpxClient(0, h.turnStore)
	bridge := &ActiveBridge{SessionID: "session-1"}
	payload := map[string]any{
		"event": "history_response",
		"items": []any{
			map[string]any{
				"kind": "tool_use", "turnId": turn.ClientRequestID, "toolName": "Write",
				"toolCallId": "c1", "input": map[string]any{"path": "a.ts", "contents": "x"},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	client.ingestTurnChangeReports(bridge, raw)

	store := h.turnStore.ChangeReports()
	first, ok, err := store.Get(turn.ID)
	if err != nil || !ok {
		t.Fatalf("first report: ok=%v err=%v", ok, err)
	}
	if first.AddedCount != 1 || first.Source != meta.TurnChangeBackfill {
		t.Fatalf("first=%+v", first)
	}

	payload["items"] = []any{
		map[string]any{
			"kind": "tool_use", "turnId": turn.ClientRequestID, "toolName": "Write",
			"toolCallId": "c2", "input": map[string]any{"path": "b.ts", "contents": "y"},
		},
	}
	raw, _ = json.Marshal(payload)
	client.ingestTurnChangeReports(bridge, raw)
	skipped, _, _ := store.Get(turn.ID)
	if skipped.AddedCount != 1 || skipped.Files[0].Path != "a.ts" {
		t.Fatalf("current recipe must skip recompute, got %+v", skipped)
	}

	rememberFinishedTurn(bridge, turn.ID)
	client.ingestTurnChangeReports(bridge, raw)
	forced, _, _ := store.Get(turn.ID)
	if forced.AddedCount != 1 || forced.Files[0].Path != "b.ts" || forced.Source != meta.TurnChangeLive {
		t.Fatalf("just-finished must recompute, got %+v", forced)
	}
	bridge.mu.Lock()
	cleared := bridge.lastFinishedTurnID
	bridge.mu.Unlock()
	if cleared != "" {
		t.Fatalf("lastFinishedTurnID should clear after ingest, got %q", cleared)
	}
}

func TestIngestTurnChangeReportsUnavailableWhenJustFinishedHasNoFiles(t *testing.T) {
	h, db, _, _, turn := mutationAttributionRig(t)
	defer db.Close()

	client := NewAcpxClient(0, h.turnStore)
	bridge := &ActiveBridge{SessionID: "session-1"}
	raw, _ := json.Marshal(map[string]any{
		"event": "history_response",
		"items": []any{
			map[string]any{"kind": "user", "turnId": turn.ID},
			map[string]any{
				"kind": "tool_use", "turnId": turn.ID, "toolName": "Read",
				"input": map[string]any{"path": "skip.ts"},
			},
		},
	})
	rememberFinishedTurn(bridge, turn.ID)
	client.ingestTurnChangeReports(bridge, raw)

	got, ok, err := h.turnStore.ChangeReports().Get(turn.ID)
	if err != nil || !ok {
		t.Fatalf("unavailable report: ok=%v err=%v", ok, err)
	}
	if got.Source != meta.TurnChangeUnavailable || len(got.Files) != 0 {
		t.Fatalf("got %+v", got)
	}
	bridge.mu.Lock()
	cleared := bridge.lastFinishedTurnID
	bridge.mu.Unlock()
	if cleared != "" {
		t.Fatalf("lastFinishedTurnID should clear after unavailable ingest, got %q", cleared)
	}
}
