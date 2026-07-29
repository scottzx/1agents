package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		clientConn.Close()
		serverConn.Close()
	})

	store := meta.NewAgentTurnStore(db)
	bridge := &ActiveBridge{
		SessionID:     "session-1",
		ProjectID:     "project-1",
		WorkspacePath: t.TempDir(),
		AgentType:     "codex",
		ServerConn:    clientConn,
		MsgChan:       make(chan []byte, 32),
		turnStore:     store,
	}
	return &AcpxClient{}, bridge, store, serverConn
}

func queueTestPrompt(t *testing.T, client *AcpxClient, bridge *ActiveBridge, requestID, text string) {
	t.Helper()
	raw, _ := json.Marshal(WsMessage{
		Action:    "prompt",
		SessionID: bridge.SessionID,
		RequestId: requestID,
		Text:      text,
	})
	handled, err := client.queuePrompt(bridge, WsMessage{
		Action:    "prompt",
		SessionID: bridge.SessionID,
		RequestId: requestID,
		Text:      text,
	}, raw)
	if err != nil || !handled {
		t.Fatalf("queuePrompt(%s): handled=%v err=%v", requestID, handled, err)
	}
}

func readForwardedPrompt(t *testing.T, conn *websocket.Conn) WsMessage {
	t.Helper()
	var msg WsMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("ReadJSON forwarded prompt: %v", err)
	}
	return msg
}

func TestTurnBridgeSerializesThreePrompts(t *testing.T) {
	client, bridge, store, runtime := newTurnBridgeTest(t)

	queueTestPrompt(t, client, bridge, "request-1", "first")
	firstPrompt := readForwardedPrompt(t, runtime)
	queueTestPrompt(t, client, bridge, "request-2", "second")
	queueTestPrompt(t, client, bridge, "request-3", "third")

	bridge.mu.Lock()
	if bridge.activeTurn == nil || len(bridge.pendingTurns) != 2 {
		t.Fatalf("active=%v pending=%d, want one active and two queued", bridge.activeTurn, len(bridge.pendingTurns))
	}
	firstID := bridge.activeTurn.Turn.ID
	secondID := bridge.pendingTurns[0].Turn.ID
	thirdID := bridge.pendingTurns[1].Turn.ID
	bridge.mu.Unlock()
	if firstPrompt.TurnID != firstID {
		t.Fatalf("first forwarded turnId=%q, want %q", firstPrompt.TurnID, firstID)
	}

	bridge.appendTurnText("answer one")
	doneRaw := []byte(`{"event":"done","summary":"one"}`)
	doneRaw, next, explicit, err := client.finishActiveTurn(
		bridge, WsMessage{Event: "done", Summary: "one"}, doneRaw, nil, nil,
	)
	if err != nil || !explicit || next == nil || next.Turn.ID != secondID {
		t.Fatalf("finish first: explicit=%v next=%+v err=%v", explicit, next, err)
	}
	var done WsMessage
	if err := json.Unmarshal(doneRaw, &done); err != nil || done.TurnID != firstID {
		t.Fatalf("done binding: msg=%+v err=%v", done, err)
	}
	if err := client.startTurn(bridge, next); err != nil {
		t.Fatalf("start second: %v", err)
	}
	if got := readForwardedPrompt(t, runtime); got.TurnID != secondID || got.Text != "second" {
		t.Fatalf("second forwarded prompt = %+v", got)
	}

	errorRaw := []byte(`{"event":"error","code":"agent_error","message":"second failed"}`)
	errorRaw, next, explicit, err = client.finishActiveTurn(
		bridge,
		WsMessage{Event: "error", Code: "agent_error", Message: "second failed"},
		errorRaw, nil, nil,
	)
	if err != nil || !explicit || next == nil || next.Turn.ID != thirdID {
		t.Fatalf("finish second: explicit=%v next=%+v err=%v", explicit, next, err)
	}
	if err := json.Unmarshal(errorRaw, &done); err != nil || done.TurnID != secondID {
		t.Fatalf("error binding: msg=%+v err=%v", done, err)
	}
	if err := client.startTurn(bridge, next); err != nil {
		t.Fatalf("start third: %v", err)
	}
	if got := readForwardedPrompt(t, runtime); got.TurnID != thirdID || got.Text != "third" {
		t.Fatalf("third forwarded prompt = %+v", got)
	}

	_, next, explicit, err = client.finishActiveTurn(
		bridge, WsMessage{Event: "done", Stopped: true}, []byte(`{"event":"done","stopped":true}`), nil, nil,
	)
	if err != nil || !explicit || next != nil {
		t.Fatalf("finish third: explicit=%v next=%+v err=%v", explicit, next, err)
	}

	want := map[string]meta.AgentTurnStatus{
		firstID:  meta.AgentTurnCompleted,
		secondID: meta.AgentTurnFailed,
		thirdID:  meta.AgentTurnCancelled,
	}
	for id, status := range want {
		turn, ok, err := store.Get(id)
		if err != nil || !ok || turn.Status != status {
			t.Fatalf("Turn %s: ok=%v status=%q err=%v, want %q", id, ok, turn.Status, err, status)
		}
	}
}

func TestTurnBridgeCancelsQueuedTurnWithoutForwarding(t *testing.T) {
	client, bridge, store, runtime := newTurnBridgeTest(t)
	queueTestPrompt(t, client, bridge, "request-1", "active")
	_ = readForwardedPrompt(t, runtime)
	queueTestPrompt(t, client, bridge, "request-2", "queued")

	bridge.mu.Lock()
	queuedID := bridge.pendingTurns[0].Turn.ID
	bridge.mu.Unlock()
	handled, err := client.cancelPendingTurn(bridge, queuedID)
	if err != nil || !handled {
		t.Fatalf("cancelPendingTurn: handled=%v err=%v", handled, err)
	}
	turn, ok, err := store.Get(queuedID)
	if err != nil || !ok || turn.Status != meta.AgentTurnCancelled {
		t.Fatalf("queued Turn after cancel: %+v ok=%v err=%v", turn, ok, err)
	}
	bridge.mu.Lock()
	pending := len(bridge.pendingTurns)
	bridge.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending Turns=%d, want 0", pending)
	}
}

func TestTurnBridgeRuntimeLossFailsActiveAndCancelsQueue(t *testing.T) {
	client, bridge, store, runtime := newTurnBridgeTest(t)
	queueTestPrompt(t, client, bridge, "request-1", "active")
	_ = readForwardedPrompt(t, runtime)
	queueTestPrompt(t, client, bridge, "request-2", "queued")

	bridge.mu.Lock()
	activeID := bridge.activeTurn.Turn.ID
	queuedID := bridge.pendingTurns[0].Turn.ID
	bridge.mu.Unlock()
	bridge.appendTurnText("partial")
	client.failOutstandingTurns(bridge, nil, nil)

	active, _, _ := store.Get(activeID)
	queued, _, _ := store.Get(queuedID)
	if active.Status != meta.AgentTurnFailed || active.ErrorCode != "runtime_lost" || active.FinalAnswer != "partial" {
		t.Fatalf("active after runtime loss: %+v", active)
	}
	if queued.Status != meta.AgentTurnCancelled || queued.ErrorCode != "runtime_lost" {
		t.Fatalf("queued after runtime loss: %+v", queued)
	}
}
