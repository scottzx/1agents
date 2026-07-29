package agent

import (
	"encoding/json"
	"fmt"
	"log"

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

func enqueueTurnState(bridge *ActiveBridge, event string, turn meta.AgentTurn) {
	raw, err := json.Marshal(map[string]any{
		"event":     event,
		"sessionId": turn.SessionID,
		"turnId":    turn.ID,
		"requestId": turn.ClientRequestID,
		"status":    turn.Status,
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
		log.Printf("[acpx_client] MsgChan full, dropping %q for Turn %s", event, turn.ID)
	}
}

// queuePrompt persists a prompt before forwarding it. It returns handled=true
// whenever the explicit Turn path is enabled, including idempotent retries.
func (c *AcpxClient) queuePrompt(bridge *ActiveBridge, msg WsMessage, raw []byte) (handled bool, err error) {
	if bridge.turnStore == nil || bridge.ProjectID == "" {
		return false, nil
	}
	turn, created, err := bridge.turnStore.Create(meta.AgentTurn{
		ProjectID:         bridge.ProjectID,
		SessionID:         bridge.SessionID,
		ClientRequestID:   msg.RequestId,
		InitiatingReplyID: bridge.ReplyID,
		AgentType:         bridge.AgentType,
		PromptText:        msg.Text,
	})
	if err != nil {
		return true, err
	}
	if !created {
		enqueueTurnState(bridge, "turn_state", turn)
		return true, nil
	}

	raw = addJSONFields(raw, map[string]any{"turnId": turn.ID})
	writeUserReply(bridge, bridge.tasksStore, msg.Text, turn.ID)
	if bridge.ReplyID != "" && bridge.tasksStore != nil {
		if err := bridge.tasksStore.SetReplyTurn(bridge.ReplyID, turn.ID); err != nil {
			log.Printf("[acpx_client] SetReplyTurn(%s, %s): %v", bridge.ReplyID, turn.ID, err)
		}
	}

	ctx := &TurnContext{Turn: turn, Raw: raw}
	bridge.mu.Lock()
	startNow := bridge.activeTurn == nil
	if startNow {
		running, transitionErr := bridge.turnStore.Transition(turn.ID, meta.AgentTurnTransition{
			Status: meta.AgentTurnRunning,
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
	}
	bridge.mu.Unlock()

	enqueueTurnState(bridge, "turn_queued", turn)
	if startNow {
		return true, c.startTurn(bridge, ctx)
	}
	return true, nil
}

func (c *AcpxClient) startTurn(bridge *ActiveBridge, ctx *TurnContext) error {
	enqueueTurnState(bridge, "turn_started", ctx.Turn)
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
	enqueueTurnState(bridge, "turn_cancelled", cancelled)
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

	text := bridge.takeTurnText()
	change := meta.AgentTurnTransition{
		Status:      meta.AgentTurnCompleted,
		FinalAnswer: text,
	}
	if msg.Event == "error" {
		change.Status = meta.AgentTurnFailed
		change.ErrorCode = msg.Code
		change.ErrorText = msg.Message
		if change.ErrorCode == "" {
			change.ErrorCode = "agent_error"
		}
	} else if msg.Stopped {
		change.Status = meta.AgentTurnCancelled
		change.ErrorCode = "cancelled_by_user"
		change.ErrorText = "Turn was stopped by the user."
	}
	finished, err := bridge.turnStore.Transition(active.Turn.ID, change)
	if err != nil {
		return raw, nil, true, err
	}
	writeAgentReplyText(bridge, tasksStore, chatStore, active.Turn.ID, text)
	raw = addJSONFields(raw, map[string]any{
		"turnId":     active.Turn.ID,
		"turnStatus": finished.Status,
	})

	bridge.mu.Lock()
	if bridge.activeTurn != nil && bridge.activeTurn.Turn.ID == active.Turn.ID {
		bridge.activeTurn = nil
	}
	var next *TurnContext
	if len(bridge.pendingTurns) > 0 {
		next = bridge.pendingTurns[0]
		bridge.pendingTurns = bridge.pendingTurns[1:]
		running, transitionErr := bridge.turnStore.Transition(next.Turn.ID, meta.AgentTurnTransition{
			Status: meta.AgentTurnRunning,
		})
		if transitionErr != nil {
			bridge.mu.Unlock()
			return raw, nil, true, transitionErr
		}
		next.Turn = running
		bridge.activeTurn = next
		bridge.turnText = nil
	}
	bridge.mu.Unlock()
	return raw, next, true, nil
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
			writeAgentReplyText(bridge, tasksStore, chatStore, active.Turn.ID, partial)
			enqueueTurnState(bridge, "turn_failed", failed)
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
		enqueueTurnState(bridge, "turn_cancelled", cancelled)
	}
}
