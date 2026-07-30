package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func newTurnBridgeTest(t *testing.T) (*AcpxClient, *ActiveBridge, *meta.AgentTurnStore, *websocket.Conn) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.EnsureProject("project-1", "Turn test", t.TempDir()); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := meta.NewSessionStore(db).Add(meta.ChatSessionRecord{
		ID:          "session-1",
		WorkspaceID: "project-1",
		AgentType:   "codex",
	}); err != nil {
		t.Fatalf("Add session: %v", err)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, upgradeErr := upgrader.Upgrade(w, r, nil)
		if upgradeErr == nil {
			serverConnCh <- conn
		}
	}))
	t.Cleanup(server.Close)
	clientConn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"), nil,
	)
	if err != nil {
		t.Fatalf("Dial test websocket: %v", err)
	}
	serverConn := <-serverConnCh
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})

	store := meta.NewAgentTurnStore(db)
	bridge := &ActiveBridge{
		SessionID:           "session-1",
		ProjectID:           "project-1",
		WorkspacePath:       t.TempDir(),
		AgentType:           "codex",
		ServerConn:          clientConn,
		MsgChan:             make(chan []byte, 32),
		turnStore:           store,
		turnProtocolVersion: 3,
	}
	client := &AcpxClient{
		bridges:   map[string]*ActiveBridge{"session-1": bridge},
		turnStore: store,
	}
	return client, bridge, store, serverConn
}

func TestQueuePromptForwardsCanonicalIdentityWithoutDatabasePrecondition(t *testing.T) {
	client, bridge, _, runtime := newTurnBridgeTest(t)
	bridge.turnStore = nil

	raw := []byte(`{"action":"prompt","sessionId":"session-1","requestId":"request-1","text":"hello"}`)
	handled, err := client.queuePrompt(bridge, WsMessage{
		Action:    "prompt",
		SessionID: bridge.SessionID,
		RequestId: "request-1",
		Text:      "hello",
	}, raw)
	if err != nil || !handled {
		t.Fatalf("queuePrompt: handled=%v err=%v", handled, err)
	}
	var forwarded WsMessage
	if err := runtime.ReadJSON(&forwarded); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if forwarded.TurnID != "" || forwarded.RequestId != "request-1" ||
		!forwarded.TurnManaged {
		t.Fatalf("forwarded identity: turnId=%q requestId=%q turnManaged=%v",
			forwarded.TurnID, forwarded.RequestId, forwarded.TurnManaged)
	}
}

func TestQueuePromptRejectsStale1ACPProtocol(t *testing.T) {
	client, bridge, _, _ := newTurnBridgeTest(t)
	bridge.turnProtocolVersion = 2
	handled, err := client.queuePrompt(
		bridge,
		WsMessage{Action: "prompt", RequestId: "request-1", Text: "hello"},
		[]byte(`{"action":"prompt","requestId":"request-1","text":"hello"}`),
	)
	if !handled || err == nil || !strings.Contains(err.Error(), "protocol v3") {
		t.Fatalf("queuePrompt stale protocol: handled=%v err=%v", handled, err)
	}
}

func TestProjectAuthoritativeTurnRebuildsLifecycleIdempotently(t *testing.T) {
	_, bridge, store, _ := newTurnBridgeTest(t)
	base := time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC)
	queued := meta.AgentTurn{
		ID:                 "turn-1",
		ProjectID:          bridge.ProjectID,
		SessionID:          bridge.SessionID,
		ClientRequestID:    "turn-1",
		AgentType:          "codex",
		Status:             meta.AgentTurnQueued,
		PromptText:         "repair projection",
		RequestFingerprint: "fingerprint",
		LastEventSeq:       1,
		CreatedAt:          base,
		UpdatedAt:          base,
	}
	if _, err := projectAuthoritativeTurn(store, queued); err != nil {
		t.Fatalf("project queued: %v", err)
	}
	startedAt := base.Add(time.Second)
	running := queued
	running.Status = meta.AgentTurnRunning
	running.StartedAt = &startedAt
	running.UpdatedAt = startedAt
	running.LastEventSeq = 2
	if _, err := projectAuthoritativeTurn(store, running); err != nil {
		t.Fatalf("project running: %v", err)
	}
	completedAt := startedAt.Add(time.Second)
	completed := running
	completed.Status = meta.AgentTurnCompleted
	completed.FinalAnswer = "done"
	completed.StopReason = "end_turn"
	completed.TerminalSource = "live_runtime"
	completed.CompletedAt = &completedAt
	completed.UpdatedAt = completedAt
	completed.LastEventSeq = 3
	if _, err := projectAuthoritativeTurn(store, completed); err != nil {
		t.Fatalf("project completed: %v", err)
	}
	if _, err := projectAuthoritativeTurn(store, completed); err != nil {
		t.Fatalf("repeat completed projection: %v", err)
	}

	stored, ok, err := store.Get("turn-1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if stored.Status != meta.AgentTurnCompleted || stored.FinalAnswer != "done" ||
		stored.TerminalSource != "live_runtime" || stored.LastEventSeq != 3 {
		t.Fatalf("stored projection: %+v", stored)
	}
}

func TestProjectAuthoritativeTerminalRebuildsMissingHistory(t *testing.T) {
	_, bridge, store, _ := newTurnBridgeTest(t)
	completedAt := time.Date(2026, 7, 29, 10, 40, 0, 0, time.UTC)
	incoming := meta.AgentTurn{
		ID:              "legacy-user-message-id",
		ProjectID:       bridge.ProjectID,
		SessionID:       bridge.SessionID,
		ClientRequestID: "legacy-user-message-id",
		AgentType:       "codex",
		Status:          meta.AgentTurnCompleted,
		PromptText:      "legacy prompt",
		FinalAnswer:     "legacy answer",
		TerminalSource:  "legacy_runtime_history",
		LastEventSeq:    1,
		CreatedAt:       completedAt.Add(-time.Minute),
		UpdatedAt:       completedAt,
		CompletedAt:     &completedAt,
	}
	if _, err := projectAuthoritativeTurn(store, incoming); err != nil {
		t.Fatalf("project terminal history: %v", err)
	}
	stored, ok, err := store.Get(incoming.ID)
	if err != nil || !ok || stored.Status != meta.AgentTurnCompleted ||
		stored.FinalAnswer != incoming.FinalAnswer {
		t.Fatalf("rebuilt Turn: %+v ok=%v err=%v", stored, ok, err)
	}
}

func TestAuthoritativeStateDrivesRunningTurnWithoutProjection(t *testing.T) {
	client, bridge, _, _ := newTurnBridgeTest(t)
	bridge.turnStore = nil
	client.turnStore = nil
	running := meta.AgentTurn{
		ID:         "turn-live",
		SessionID:  bridge.SessionID,
		ProjectID:  bridge.ProjectID,
		AgentType:  "codex",
		Status:     meta.AgentTurnRunning,
		PromptText: "live",
	}
	client.acceptAuthoritativeTurn(bridge, running, false)

	got, ok, authoritative := client.authoritativeRunningTurn(bridge.SessionID)
	if !authoritative || !ok || got.ID != running.ID {
		t.Fatalf("authoritative running Turn: %+v ok=%v authoritative=%v", got, ok, authoritative)
	}
}

func TestTerminalProjectionFailureDoesNotLeaveLiveTurnActive(t *testing.T) {
	client, bridge, _, _ := newTurnBridgeTest(t)
	bridge.turnStore = nil
	client.turnStore = nil
	running := meta.AgentTurn{
		ID:        "turn-live",
		SessionID: bridge.SessionID,
		ProjectID: bridge.ProjectID,
		AgentType: "codex",
		Status:    meta.AgentTurnRunning,
	}
	client.acceptAuthoritativeTurn(bridge, running, false)

	raw := []byte(`{"event":"turn_terminal","turnId":"turn-live","status":"completed","finalAnswer":"done","journalSequence":2}`)
	var msg WsMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, explicit := client.finishActiveTurn(bridge, msg, raw, nil, nil); !explicit {
		t.Fatal("terminal event was not consumed")
	}
	if _, ok, authoritative := client.authoritativeRunningTurn(bridge.SessionID); !authoritative || ok {
		t.Fatalf("running Turn after terminal: ok=%v authoritative=%v", ok, authoritative)
	}
}
