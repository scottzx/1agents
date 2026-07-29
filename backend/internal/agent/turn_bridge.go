package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func addJSONFields(raw []byte, fields map[string]any) []byte {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	for key, value := range fields {
		payload[key] = value
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return updated
}

func enqueueTurnState(bridge *ActiveBridge, event string, turn meta.AgentTurn, queuePosition ...int) {
	payload := map[string]any{
		"event":      event,
		"sessionId":  turn.SessionID,
		"turnId":     turn.ID,
		"requestId":  turn.ClientRequestID,
		"status":     turn.Status,
		"promptText": turn.PromptText,
		"createdAt":  turn.CreatedAt,
	}
	if len(queuePosition) > 0 && queuePosition[0] > 0 {
		payload["queuePosition"] = queuePosition[0]
	}
	if turn.StartedAt != nil {
		payload["startedAt"] = turn.StartedAt
	}
	if turn.CompletedAt != nil {
		payload["completedAt"] = turn.CompletedAt
	}
	if turn.StopReason != "" {
		payload["stopReason"] = turn.StopReason
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.IsDone {
		return
	}
	select {
	case bridge.MsgChan <- raw:
	default:
		log.Printf("[acpx_client] MsgChan full, dropping %q for Turn %s", event, turn.ID)
	}
}

func queuedTurnPosition(store *meta.AgentTurnStore, sessionID, turnID string) int {
	queued, err := store.QueuedBySession(sessionID)
	if err != nil {
		return 0
	}
	for index, candidate := range queued {
		if candidate.ID == turnID {
			return index + 1
		}
	}
	return 0
}

func enqueueProtocolError(bridge *ActiveBridge, requestID, code, message string) {
	raw, err := json.Marshal(map[string]any{
		"event":     "protocol_error",
		"sessionId": bridge.SessionID,
		"requestId": requestID,
		"code":      code,
		"message":   message,
	})
	if err != nil {
		return
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.IsDone {
		return
	}
	select {
	case bridge.MsgChan <- raw:
	default:
		log.Printf("[acpx_client] MsgChan full, dropping protocol_error for %s", bridge.SessionID)
	}
}

func promptFingerprint(msg WsMessage) string {
	canonical, _ := json.Marshal(struct {
		Text        string          `json:"text"`
		Attachments json.RawMessage `json:"attachments,omitempty"`
	}{
		Text:        msg.Text,
		Attachments: msg.Attachments,
	})
	return fmt.Sprintf("%x", sha256.Sum256(canonical))
}

func legacyRequestID() string {
	return fmt.Sprintf("legacy-%d", time.Now().UTC().UnixNano())
}

// queuePrompt persists a prompt before forwarding it. It returns handled=true
// whenever the explicit Turn path is enabled, including idempotent retries.
func (c *AcpxClient) queuePrompt(bridge *ActiveBridge, msg WsMessage, raw []byte) (handled bool, err error) {
	if bridge.turnStore == nil || bridge.ProjectID == "" {
		return false, nil
	}
	if msg.TurnID != "" {
		return true, fmt.Errorf("browser supplied forbidden turnId")
	}
	if msg.RequestId == "" {
		msg.RequestId = legacyRequestID()
		raw = addJSONFields(raw, map[string]any{"requestId": msg.RequestId})
	}
	turn, created, err := bridge.turnStore.Create(meta.AgentTurn{
		ProjectID:          bridge.ProjectID,
		SessionID:          bridge.SessionID,
		ClientRequestID:    msg.RequestId,
		AgentType:          bridge.AgentType,
		PromptText:         msg.Text,
		RequestFingerprint: promptFingerprint(msg),
	})
	if err != nil {
		return true, err
	}
	if !created {
		enqueueTurnState(bridge, "turn_state", turn, queuedTurnPosition(bridge.turnStore, bridge.SessionID, turn.ID))
		return true, nil
	}

	raw = addJSONFields(raw, map[string]any{
		"turnId":    turn.ID,
		"requestId": turn.ID,
	})
	initiatingReplyID := writeUserReply(bridge, bridge.tasksStore, msg.Text, turn.ID)
	if initiatingReplyID != "" {
		if err := bridge.turnStore.SetReplyLinks(turn.ID, initiatingReplyID, ""); err != nil {
			log.Printf("[acpx_client] Set initiating Reply for Turn %s: %v", turn.ID, err)
		} else {
			turn.InitiatingReplyID = initiatingReplyID
		}
	}

	ctx := &TurnContext{Turn: turn, Raw: raw}
	queuePosition := 0
	bridge.mu.Lock()
	startNow := bridge.activeTurn == nil
	if startNow {
		running, transitionErr := bridge.turnStore.Transition(turn.ID, meta.AgentTurnTransition{
			Status:           meta.AgentTurnRunning,
			RuntimeRecordID:  bridge.SessionID,
			RuntimeRequestID: turn.ID,
			PromptMessageID:  turn.ID,
		})
		if transitionErr != nil {
			bridge.mu.Unlock()
			return true, transitionErr
		}
		ctx.Turn = running
		bridge.activeTurn = ctx
		bridge.turnText = nil
	} else {
		bridge.pendingTurns = append(bridge.pendingTurns, ctx)
		queuePosition = len(bridge.pendingTurns)
	}
	bridge.mu.Unlock()

	enqueueTurnState(bridge, "turn_state", turn, queuePosition)
	if startNow {
		return true, c.startTurn(bridge, ctx)
	}
	return true, nil
}

func (c *AcpxClient) startTurn(bridge *ActiveBridge, ctx *TurnContext) error {
	enqueueTurnState(bridge, "turn_state", ctx.Turn)
	return c.writeServerRaw(bridge, ctx.Raw)
}

// writeServerRaw serializes every bridge-server write. The runtime reader can
// promote a queued Turn while the client reader forwards controls such as
// cancel_turn, and gorilla/websocket permits only one concurrent writer.
func (c *AcpxClient) writeServerRaw(bridge *ActiveBridge, raw []byte) error {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	serverConn := bridge.ServerConn
	isDone := bridge.IsDone
	if isDone || serverConn == nil {
		return fmt.Errorf("ACP runtime connection is unavailable")
	}
	return serverConn.WriteMessage(websocket.TextMessage, raw)
}

// cancelPendingTurn consumes a targeted cancel for a queued Turn. Active Turn
// cancellation is still forwarded to ACP and reaches a stopped `done` event.
func (c *AcpxClient) cancelPendingTurn(bridge *ActiveBridge, turnID string) (bool, error) {
	if turnID == "" || bridge.turnStore == nil {
		return false, nil
	}
	bridge.mu.Lock()
	if bridge.activeTurn != nil && bridge.activeTurn.Turn.ID == turnID {
		bridge.mu.Unlock()
		return false, nil
	}
	var target *TurnContext
	for i, pending := range bridge.pendingTurns {
		if pending.Turn.ID == turnID {
			target = pending
			bridge.pendingTurns = append(bridge.pendingTurns[:i], bridge.pendingTurns[i+1:]...)
			break
		}
	}
	bridge.mu.Unlock()
	if target == nil {
		turn, ok, err := bridge.turnStore.Get(turnID)
		if err != nil || !ok || turn.SessionID != bridge.SessionID {
			return false, err
		}
		enqueueTurnState(bridge, "turn_state", turn)
		return true, nil
	}
	cancelled, err := bridge.turnStore.Transition(turnID, meta.AgentTurnTransition{
		Status:    meta.AgentTurnCancelled,
		ErrorCode: "cancelled_by_user",
		ErrorText: "Queued Turn was cancelled before execution.",
	})
	if err != nil {
		return true, err
	}
	enqueueTurnState(bridge, "turn_state", cancelled)
	return true, nil
}

// finishActiveTurn applies one runtime terminal event to exactly the active
// Turn, then reserves the FIFO queue head as the next running Turn.
func (c *AcpxClient) finishActiveTurn(bridge *ActiveBridge, msg WsMessage, raw []byte, tasksStore *TasksStore, chatStore *Store) ([]byte, *TurnContext, bool, error) {
	bridge.mu.Lock()
	active := bridge.activeTurn
	bridge.mu.Unlock()
	if active == nil || bridge.turnStore == nil {
		return raw, nil, false, nil
	}
	if msg.Event != "turn_terminal" {
		return raw, nil, false, nil
	}
	if msg.TurnID == "" || msg.TurnID != active.Turn.ID {
		log.Printf("[acpx_client] Ignoring terminal mismatch: session_id=%s active_turn_id=%s event_turn_id=%s",
			bridge.SessionID, active.Turn.ID, msg.TurnID)
		return raw, nil, false, nil
	}

	text := msg.FinalAnswer
	change := meta.AgentTurnTransition{
		Status:           meta.AgentTurnCompleted,
		FinalAnswer:      text,
		RuntimeRecordID:  bridge.SessionID,
		RuntimeRequestID: msg.RuntimeRequestID,
		PromptMessageID:  msg.PromptMessageID,
		StopReason:       msg.StopReason,
		TerminalSource:   "live_runtime",
		LastEventSeq:     msg.Sequence,
	}
	switch meta.AgentTurnStatus(msg.Status) {
	case meta.AgentTurnFailed:
		change.Status = meta.AgentTurnFailed
		if msg.Error != nil {
			change.ErrorCode = msg.Error.Code
			change.ErrorText = msg.Error.Message
		}
		if change.ErrorCode == "" {
			change.ErrorCode = "runtime_error"
		}
	case meta.AgentTurnCancelled:
		change.Status = meta.AgentTurnCancelled
		change.ErrorCode = "cancelled_by_user"
	case meta.AgentTurnCompleted:
	default:
		return raw, nil, true, fmt.Errorf("invalid turn terminal status %q", msg.Status)
	}
	finished, err := bridge.turnStore.Transition(active.Turn.ID, change)
	if err != nil {
		return raw, nil, true, err
	}
	finalReplyID := writeAgentReplyText(
		bridge, tasksStore, chatStore, active.Turn.ID, active.Turn.InitiatingReplyID, text,
	)
	if finalReplyID != "" {
		if err := bridge.turnStore.SetReplyLinks(active.Turn.ID, "", finalReplyID); err != nil {
			log.Printf("[acpx_client] Set final Reply for Turn %s: %v", active.Turn.ID, err)
		}
	}
	raw = addJSONFields(raw, map[string]any{"turnStatus": finished.Status})

	var next *TurnContext
	nextTurn, ok, nextErr := bridge.turnStore.NextQueued(bridge.SessionID)
	if nextErr != nil {
		return raw, nil, true, nextErr
	}
	bridge.mu.Lock()
	if ok {
		for i, pending := range bridge.pendingTurns {
			if pending.Turn.ID == nextTurn.ID {
				next = pending
				bridge.pendingTurns = append(bridge.pendingTurns[:i], bridge.pendingTurns[i+1:]...)
				break
			}
		}
	}
	if next != nil {
		running, transitionErr := bridge.turnStore.Transition(next.Turn.ID, meta.AgentTurnTransition{
			Status:           meta.AgentTurnRunning,
			RuntimeRecordID:  bridge.SessionID,
			RuntimeRequestID: next.Turn.ID,
			PromptMessageID:  next.Turn.ID,
		})
		if transitionErr != nil {
			bridge.mu.Unlock()
			return raw, nil, true, transitionErr
		}
		next.Turn = running
		bridge.activeTurn = next
		bridge.turnText = nil
	} else if bridge.activeTurn != nil && bridge.activeTurn.Turn.ID == active.Turn.ID {
		bridge.activeTurn = nil
	}
	bridge.mu.Unlock()
	return raw, next, true, nil
}

func (c *AcpxClient) enqueueTurnSync(bridge *ActiveBridge) {
	if bridge.turnStore == nil {
		return
	}
	queued, err := bridge.turnStore.QueuedBySession(bridge.SessionID)
	if err != nil {
		log.Printf("[acpx_client] turn sync queue for %s: %v", bridge.SessionID, err)
		return
	}
	bridge.mu.Lock()
	active := bridge.activeTurn
	bridge.mu.Unlock()
	payload := map[string]any{
		"event":     "turn_sync",
		"sessionId": bridge.SessionID,
		"queued":    queued,
	}
	if active != nil {
		payload["active"] = active.Turn
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.IsDone {
		return
	}
	select {
	case bridge.MsgChan <- raw:
	default:
		log.Printf("[acpx_client] MsgChan full, dropping turn_sync for %s", bridge.SessionID)
	}
}

func (c *AcpxClient) failOutstandingTurns(bridge *ActiveBridge, tasksStore *TasksStore, chatStore *Store) {
	if bridge.turnStore == nil {
		return
	}
	partial := bridge.takeTurnText()
	bridge.mu.Lock()
	active := bridge.activeTurn
	pending := append([]*TurnContext(nil), bridge.pendingTurns...)
	bridge.activeTurn = nil
	bridge.pendingTurns = nil
	bridge.mu.Unlock()

	if active != nil {
		failed, err := bridge.turnStore.Transition(active.Turn.ID, meta.AgentTurnTransition{
			Status:      meta.AgentTurnFailed,
			FinalAnswer: partial,
			ErrorCode:   "runtime_lost",
			ErrorText:   "ACP runtime connection ended before the Turn completed.",
		})
		if err != nil {
			log.Printf("[acpx_client] Fail active Turn %s after runtime loss: %v", active.Turn.ID, err)
		} else {
			finalReplyID := writeAgentReplyText(
				bridge, tasksStore, chatStore, active.Turn.ID, active.Turn.InitiatingReplyID, partial,
			)
			if finalReplyID != "" {
				_ = bridge.turnStore.SetReplyLinks(active.Turn.ID, "", finalReplyID)
			}
			enqueueTurnState(bridge, "turn_state", failed)
		}
	}
	for _, queued := range pending {
		cancelled, err := bridge.turnStore.Transition(queued.Turn.ID, meta.AgentTurnTransition{
			Status:    meta.AgentTurnCancelled,
			ErrorCode: "runtime_lost",
			ErrorText: "ACP runtime connection ended before this queued Turn started.",
		})
		if err != nil {
			log.Printf("[acpx_client] Cancel queued Turn %s after runtime loss: %v", queued.Turn.ID, err)
			continue
		}
		enqueueTurnState(bridge, "turn_state", cancelled)
	}
}
