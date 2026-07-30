package agent

import (
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

func removeJSONFields(raw []byte, fields ...string) []byte {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	for _, field := range fields {
		delete(payload, field)
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return updated
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

func legacyRequestID() string {
	return fmt.Sprintf("legacy-%d", time.Now().UTC().UnixNano())
}

// queuePrompt forwards the browser requestId as an idempotency key and marks
// the request as managed. 1ACP generates the trusted Turn ID and persists the
// Journal before it acknowledges or executes the prompt; agent_turns is only a
// rebuildable projection of those facts.
func (c *AcpxClient) queuePrompt(bridge *ActiveBridge, msg WsMessage, raw []byte) (handled bool, err error) {
	if msg.TurnID != "" {
		return true, fmt.Errorf("browser supplied forbidden turnId")
	}
	bridge.mu.Lock()
	protocolVersion := bridge.turnProtocolVersion
	bridge.mu.Unlock()
	if protocolVersion < 3 {
		return true, fmt.Errorf(
			"1ACP Turn protocol v3 is required; restart the updated 1ACP bridge before sending prompts",
		)
	}
	clientRequestID := msg.RequestId
	if clientRequestID == "" {
		clientRequestID = legacyRequestID()
	}
	raw = addJSONFields(raw, map[string]any{
		"requestId":   clientRequestID,
		"turnManaged": true,
	})
	return true, c.writeServerRaw(bridge, raw)
}

// writeServerRaw serializes every bridge-server write. gorilla/websocket
// permits only one concurrent writer.
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

func turnFromMessage(bridge *ActiveBridge, msg WsMessage) meta.AgentTurn {
	turnID := msg.TurnID
	if turnID == "" {
		turnID = msg.ID
	}
	clientRequestID := msg.ClientRequestID
	if clientRequestID == "" {
		clientRequestID = msg.RequestId
	}
	sequence := msg.LastEventSeq
	if sequence == 0 {
		sequence = msg.JournalSequence
	}
	errorCode := msg.ErrorCode
	errorText := msg.ErrorText
	if msg.Error != nil {
		if errorCode == "" {
			errorCode = msg.Error.Code
		}
		if errorText == "" {
			errorText = msg.Error.Message
		}
	}
	return meta.AgentTurn{
		ID:                 turnID,
		ProjectID:          bridge.ProjectID,
		SessionID:          bridge.SessionID,
		ClientRequestID:    clientRequestID,
		AgentType:          firstNonEmpty(msg.AgentType, bridge.AgentType),
		Status:             meta.AgentTurnStatus(msg.Status),
		PromptText:         msg.PromptText,
		RequestFingerprint: msg.RequestFingerprint,
		FinalAnswer:        msg.FinalAnswer,
		ErrorCode:          errorCode,
		ErrorText:          errorText,
		RuntimeRecordID:    msg.RuntimeRecordID,
		RuntimeRequestID:   msg.RuntimeRequestID,
		PromptMessageID:    msg.PromptMessageID,
		StopReason:         msg.StopReason,
		TerminalSource:     msg.TerminalSource,
		LastEventSeq:       sequence,
		StartedAt:          msg.StartedAt,
		CompletedAt:        msg.CompletedAt,
		CreatedAt:          msg.CreatedAt,
		UpdatedAt:          msg.UpdatedAt,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func terminalTurnStatus(status meta.AgentTurnStatus) bool {
	return status == meta.AgentTurnCompleted ||
		status == meta.AgentTurnFailed ||
		status == meta.AgentTurnCancelled
}

// projectAuthoritativeTurn applies one 1ACP Journal snapshot to SQLite. It is
// deliberately idempotent and must never be used to decide whether execution
// may proceed.
func projectAuthoritativeTurn(store *meta.AgentTurnStore, incoming meta.AgentTurn) (meta.AgentTurn, error) {
	if store == nil {
		return incoming, nil
	}
	if incoming.ID == "" || incoming.ProjectID == "" || incoming.SessionID == "" {
		return incoming, fmt.Errorf("incomplete authoritative Turn projection")
	}
	switch incoming.Status {
	case meta.AgentTurnQueued, meta.AgentTurnRunning, meta.AgentTurnCompleted,
		meta.AgentTurnFailed, meta.AgentTurnCancelled:
	default:
		return incoming, fmt.Errorf("invalid authoritative Turn status %q", incoming.Status)
	}

	current, ok, err := store.Get(incoming.ID)
	if err != nil {
		return incoming, err
	}
	if !ok {
		createdAt := incoming.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		current, _, err = store.Create(meta.AgentTurn{
			ID:                 incoming.ID,
			ProjectID:          incoming.ProjectID,
			SessionID:          incoming.SessionID,
			ClientRequestID:    firstNonEmpty(incoming.ClientRequestID, incoming.ID),
			AgentType:          incoming.AgentType,
			PromptText:         incoming.PromptText,
			RequestFingerprint: incoming.RequestFingerprint,
			RuntimeRecordID:    incoming.RuntimeRecordID,
			LastEventSeq:       incoming.LastEventSeq,
			CreatedAt:          createdAt,
		})
		if err != nil {
			return incoming, err
		}
	}

	if current.Status == incoming.Status || terminalTurnStatus(current.Status) {
		return current, nil
	}
	if incoming.Status == meta.AgentTurnQueued {
		return current, nil
	}
	if current.Status == meta.AgentTurnQueued && incoming.Status != meta.AgentTurnCancelled {
		at := incoming.UpdatedAt
		if incoming.StartedAt != nil {
			at = *incoming.StartedAt
		}
		current, err = store.Transition(current.ID, meta.AgentTurnTransition{
			Status:           meta.AgentTurnRunning,
			RuntimeRecordID:  incoming.RuntimeRecordID,
			RuntimeRequestID: incoming.RuntimeRequestID,
			PromptMessageID:  incoming.PromptMessageID,
			LastEventSeq:     incoming.LastEventSeq,
			At:               at,
		})
		if err != nil {
			return incoming, err
		}
	}
	if incoming.Status == meta.AgentTurnRunning {
		return current, nil
	}

	at := incoming.UpdatedAt
	if incoming.CompletedAt != nil {
		at = *incoming.CompletedAt
	}
	return store.Transition(current.ID, meta.AgentTurnTransition{
		Status:           incoming.Status,
		FinalAnswer:      incoming.FinalAnswer,
		ErrorCode:        incoming.ErrorCode,
		ErrorText:        incoming.ErrorText,
		RuntimeRecordID:  incoming.RuntimeRecordID,
		RuntimeRequestID: incoming.RuntimeRequestID,
		PromptMessageID:  incoming.PromptMessageID,
		StopReason:       incoming.StopReason,
		TerminalSource:   incoming.TerminalSource,
		LastEventSeq:     incoming.LastEventSeq,
		At:               at,
	})
}

func findTurnContext(turns []*TurnContext, turnID string) (*TurnContext, int) {
	for index, ctx := range turns {
		if ctx.Turn.ID == turnID {
			return ctx, index
		}
	}
	return nil, -1
}

func mergeTurnContext(existing *TurnContext, turn meta.AgentTurn) *TurnContext {
	if existing == nil {
		return &TurnContext{Turn: turn}
	}
	if existing.Turn.InitiatingReplyID != "" && turn.InitiatingReplyID == "" {
		turn.InitiatingReplyID = existing.Turn.InitiatingReplyID
	}
	existing.Turn = turn
	return existing
}

// acceptAuthoritativeTurn updates the live in-memory state before attempting
// the SQLite projection. MCP attribution therefore remains available when the
// projection database is locked or temporarily unavailable.
func (c *AcpxClient) acceptAuthoritativeTurn(bridge *ActiveBridge, turn meta.AgentTurn, acceptedNew bool) {
	if turn.ID == "" {
		return
	}

	bridge.mu.Lock()
	bridge.turnSyncReceived = true
	active := bridge.activeTurn
	pending, pendingIndex := findTurnContext(bridge.pendingTurns, turn.ID)
	switch turn.Status {
	case meta.AgentTurnQueued:
		if active == nil || active.Turn.ID != turn.ID {
			if pending == nil {
				bridge.pendingTurns = append(bridge.pendingTurns, &TurnContext{Turn: turn})
			} else {
				mergeTurnContext(pending, turn)
			}
		}
	case meta.AgentTurnRunning:
		if pending != nil {
			bridge.pendingTurns = append(bridge.pendingTurns[:pendingIndex], bridge.pendingTurns[pendingIndex+1:]...)
		}
		if active == nil || active.Turn.ID != turn.ID {
			bridge.activeTurn = mergeTurnContext(pending, turn)
			bridge.turnText = nil
		} else {
			mergeTurnContext(active, turn)
		}
	default:
		if pending != nil {
			bridge.pendingTurns = append(bridge.pendingTurns[:pendingIndex], bridge.pendingTurns[pendingIndex+1:]...)
		}
		if active != nil && active.Turn.ID == turn.ID {
			mergeTurnContext(active, turn)
		}
	}
	bridge.mu.Unlock()

	go func() {
		bridge.projectionMu.Lock()
		defer bridge.projectionMu.Unlock()
		if acceptedNew {
			initiatingReplyID := writeUserReply(bridge, bridge.tasksStore, turn.PromptText, turn.ID)
			if initiatingReplyID != "" {
				bridge.mu.Lock()
				if bridge.activeTurn != nil && bridge.activeTurn.Turn.ID == turn.ID {
					bridge.activeTurn.Turn.InitiatingReplyID = initiatingReplyID
				} else if ctx, _ := findTurnContext(bridge.pendingTurns, turn.ID); ctx != nil {
					ctx.Turn.InitiatingReplyID = initiatingReplyID
				}
				bridge.mu.Unlock()
				turn.InitiatingReplyID = initiatingReplyID
			}
		}

		projected, err := projectAuthoritativeTurn(bridge.turnStore, turn)
		if err != nil {
			log.Printf("[acpx_client] Non-blocking Turn projection failed: session_id=%s turn_id=%s status=%s err=%v",
				bridge.SessionID, turn.ID, turn.Status, err)
			return
		}
		if acceptedNew && turn.InitiatingReplyID != "" && bridge.turnStore != nil {
			if err := bridge.turnStore.SetReplyLinks(projected.ID, turn.InitiatingReplyID, ""); err != nil {
				log.Printf("[acpx_client] Non-blocking initiating Reply projection failed for Turn %s: %v", turn.ID, err)
			}
		}
	}()
}

func (c *AcpxClient) acceptAuthoritativeSync(bridge *ActiveBridge, msg WsMessage) {
	all := msg.Turns
	if len(all) == 0 {
		if msg.Active != nil {
			all = append(all, *msg.Active)
		}
		all = append(all, msg.Queued...)
	}

	var active *TurnContext
	if msg.Active != nil {
		turn := *msg.Active
		turn.ProjectID = bridge.ProjectID
		turn.SessionID = bridge.SessionID
		if turn.AgentType == "" {
			turn.AgentType = bridge.AgentType
		}
		active = &TurnContext{Turn: turn}
	}
	pending := make([]*TurnContext, 0, len(msg.Queued))
	for _, queued := range msg.Queued {
		queued.ProjectID = bridge.ProjectID
		queued.SessionID = bridge.SessionID
		if queued.AgentType == "" {
			queued.AgentType = bridge.AgentType
		}
		pending = append(pending, &TurnContext{Turn: queued})
	}
	bridge.mu.Lock()
	bridge.turnSyncReceived = true
	bridge.activeTurn = active
	bridge.pendingTurns = pending
	if active == nil {
		bridge.turnText = nil
	}
	bridge.mu.Unlock()

	go c.projectAuthoritativeSync(bridge, all)
}

func (c *AcpxClient) projectAuthoritativeSync(bridge *ActiveBridge, all []meta.AgentTurn) {
	bridge.projectionMu.Lock()
	defer bridge.projectionMu.Unlock()
	if bridge.turnStore != nil {
		known := make(map[string]bool, len(all))
		for _, turn := range all {
			known[turn.ID] = true
		}
		if stale, ok, err := bridge.turnStore.RunningBySession(bridge.SessionID); err != nil {
			log.Printf("[acpx_client] Non-blocking stale running projection lookup failed for %s: %v",
				bridge.SessionID, err)
		} else if ok && !known[stale.ID] {
			if _, err := bridge.turnStore.Transition(stale.ID, meta.AgentTurnTransition{
				Status:         meta.AgentTurnFailed,
				ErrorCode:      "missing_from_turn_journal",
				ErrorText:      "Turn was absent from the authoritative 1ACP Journal snapshot.",
				StopReason:     "projection_reconciled",
				TerminalSource: "turn_journal_sync",
			}); err != nil {
				log.Printf("[acpx_client] Non-blocking stale running projection cleanup failed for Turn %s: %v",
					stale.ID, err)
			}
		}
		if staleQueued, err := bridge.turnStore.QueuedBySession(bridge.SessionID); err != nil {
			log.Printf("[acpx_client] Non-blocking stale queue projection lookup failed for %s: %v",
				bridge.SessionID, err)
		} else {
			for _, stale := range staleQueued {
				if known[stale.ID] {
					continue
				}
				if _, err := bridge.turnStore.Transition(stale.ID, meta.AgentTurnTransition{
					Status:         meta.AgentTurnCancelled,
					ErrorCode:      "missing_from_turn_journal",
					ErrorText:      "Queued Turn was absent from the authoritative 1ACP Journal snapshot.",
					StopReason:     "projection_reconciled",
					TerminalSource: "turn_journal_sync",
				}); err != nil {
					log.Printf("[acpx_client] Non-blocking stale queue projection cleanup failed for Turn %s: %v",
						stale.ID, err)
				}
			}
		}
	}
	for _, turn := range all {
		turn.ProjectID = bridge.ProjectID
		turn.SessionID = bridge.SessionID
		if turn.AgentType == "" {
			turn.AgentType = bridge.AgentType
		}
		projected, err := projectAuthoritativeTurn(bridge.turnStore, turn)
		if err != nil {
			log.Printf("[acpx_client] Non-blocking Turn sync projection failed: session_id=%s turn_id=%s status=%s err=%v",
				bridge.SessionID, turn.ID, turn.Status, err)
			projected = turn
		}
		initiatingReplyID := projected.InitiatingReplyID
		if initiatingReplyID == "" {
			initiatingReplyID = writeUserReply(bridge, bridge.tasksStore, turn.PromptText, turn.ID)
			if initiatingReplyID != "" && bridge.turnStore != nil {
				if err := bridge.turnStore.SetReplyLinks(turn.ID, initiatingReplyID, ""); err != nil {
					log.Printf("[acpx_client] Non-blocking synced initiating Reply projection failed for Turn %s: %v",
						turn.ID, err)
				}
			}
		}
		if terminalTurnStatus(turn.Status) && turn.FinalAnswer != "" {
			finalReplyID := writeAgentReplyText(
				bridge, bridge.tasksStore, bridge.chatStore, turn.ID, initiatingReplyID, turn.FinalAnswer,
			)
			if finalReplyID != "" && bridge.turnStore != nil {
				if err := bridge.turnStore.SetReplyLinks(turn.ID, "", finalReplyID); err != nil {
					log.Printf("[acpx_client] Non-blocking synced final Reply projection failed for Turn %s: %v",
						turn.ID, err)
				}
			}
		}
	}
}

// finishActiveTurn consumes a terminal fact already persisted by 1ACP. SQLite
// and task-timeline linkage are best-effort projections and cannot terminate
// the runtime reader loop.
func (c *AcpxClient) finishActiveTurn(bridge *ActiveBridge, msg WsMessage, raw []byte, tasksStore *TasksStore, chatStore *Store) ([]byte, bool) {
	if msg.Event != "turn_terminal" || msg.TurnID == "" {
		return raw, false
	}
	turn := turnFromMessage(bridge, msg)

	bridge.mu.Lock()
	active := bridge.activeTurn
	if active != nil && active.Turn.ID == turn.ID {
		if active.Turn.InitiatingReplyID != "" {
			turn.InitiatingReplyID = active.Turn.InitiatingReplyID
		}
		bridge.activeTurn = nil
	}
	if pending, index := findTurnContext(bridge.pendingTurns, turn.ID); pending != nil {
		if turn.InitiatingReplyID == "" {
			turn.InitiatingReplyID = pending.Turn.InitiatingReplyID
		}
		bridge.pendingTurns = append(bridge.pendingTurns[:index], bridge.pendingTurns[index+1:]...)
	}
	bridge.turnSyncReceived = true
	bridge.mu.Unlock()

	go func() {
		bridge.projectionMu.Lock()
		defer bridge.projectionMu.Unlock()
		projected, err := projectAuthoritativeTurn(bridge.turnStore, turn)
		if err != nil {
			log.Printf("[acpx_client] Non-blocking terminal projection failed: session_id=%s turn_id=%s err=%v",
				bridge.SessionID, turn.ID, err)
		} else if turn.InitiatingReplyID == "" {
			turn.InitiatingReplyID = projected.InitiatingReplyID
		}

		if turn.InitiatingReplyID == "" {
			turn.InitiatingReplyID = writeUserReply(
				bridge, tasksStore, turn.PromptText, turn.ID,
			)
		}
		finalReplyID := writeAgentReplyText(
			bridge, tasksStore, chatStore, turn.ID, turn.InitiatingReplyID, turn.FinalAnswer,
		)
		if bridge.turnStore != nil {
			if turn.InitiatingReplyID != "" {
				if err := bridge.turnStore.SetReplyLinks(turn.ID, turn.InitiatingReplyID, ""); err != nil {
					log.Printf("[acpx_client] Non-blocking terminal initiating Reply projection failed for Turn %s: %v",
						turn.ID, err)
				}
			}
			if finalReplyID != "" {
				if err := bridge.turnStore.SetReplyLinks(turn.ID, "", finalReplyID); err != nil {
					log.Printf("[acpx_client] Non-blocking final Reply projection failed for Turn %s: %v", turn.ID, err)
				}
			}
		}
	}()
	raw = addJSONFields(raw, map[string]any{"turnStatus": turn.Status})
	return raw, true
}

func (c *AcpxClient) authoritativeRunningTurn(sessionID string) (meta.AgentTurn, bool, bool) {
	c.mu.Lock()
	bridge := c.bridges[sessionID]
	known := c.turnStateKnown[sessionID]
	c.mu.Unlock()
	if bridge == nil {
		return meta.AgentTurn{}, false, known
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if !bridge.turnSyncReceived {
		// A Browser bridge is already waiting for its mandatory 1ACP sync.
		// Do not trust a possibly stale SQLite running row during this window.
		return meta.AgentTurn{}, false, true
	}
	if bridge.activeTurn == nil || bridge.activeTurn.Turn.Status != meta.AgentTurnRunning {
		return meta.AgentTurn{}, false, true
	}
	return bridge.activeTurn.Turn, true, true
}

func (c *AcpxClient) clearAuthoritativeTurns(bridge *ActiveBridge) {
	bridge.mu.Lock()
	bridge.activeTurn = nil
	bridge.pendingTurns = nil
	bridge.turnSyncReceived = true
	bridge.mu.Unlock()
	c.mu.Lock()
	if c.turnStateKnown == nil {
		c.turnStateKnown = make(map[string]bool)
	}
	c.turnStateKnown[bridge.SessionID] = true
	c.mu.Unlock()
}
