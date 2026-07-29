package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

type runtimeTurnSnapshot struct {
	Status          meta.AgentTurnStatus `json:"status"`
	PromptMessageID string               `json:"prompt_message_id"`
	StopReason      string               `json:"stop_reason"`
	LastEventSeq    int64                `json:"last_event_seq"`
	ErrorCode       string               `json:"error_code"`
}

type runtimeSessionRecord struct {
	Messages []json.RawMessage `json:"messages"`
	Acpx     struct {
		TurnResults map[string]runtimeTurnSnapshot `json:"turn_results"`
	} `json:"acpx"`
}

func defaultRuntimeStateDir() string {
	home := os.Getenv("ONEAGENTS_HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".1agents", "acpx-state", "sessions")
}

func runtimeRecordPath(stateDir, sessionID string) string {
	safeID := strings.ReplaceAll(url.QueryEscape(sessionID), "+", "%20")
	return filepath.Join(stateDir, safeID+".json")
}

func loadRuntimeSessionRecord(stateDir, sessionID string) (runtimeSessionRecord, error) {
	raw, err := os.ReadFile(runtimeRecordPath(stateDir, sessionID))
	if err != nil {
		return runtimeSessionRecord{}, err
	}
	var record runtimeSessionRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return runtimeSessionRecord{}, err
	}
	return record, nil
}

func finalAnswerForTurn(messages []json.RawMessage, turnID string) string {
	sawTurn := false
	finalAnswer := ""
	for _, raw := range messages {
		var message map[string]json.RawMessage
		if json.Unmarshal(raw, &message) != nil {
			continue
		}
		if userRaw, ok := message["User"]; ok {
			var user struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(userRaw, &user) == nil {
				if sawTurn {
					break
				}
				sawTurn = user.ID == turnID
			}
			continue
		}
		if !sawTurn {
			continue
		}
		agentRaw, ok := message["Agent"]
		if !ok {
			continue
		}
		var agent struct {
			Content []map[string]json.RawMessage `json:"content"`
		}
		if json.Unmarshal(agentRaw, &agent) != nil {
			continue
		}
		var text strings.Builder
		for _, content := range agent.Content {
			if textRaw, ok := content["Text"]; ok {
				var chunk string
				if json.Unmarshal(textRaw, &chunk) == nil {
					text.WriteString(chunk)
				}
			}
		}
		if text.Len() > 0 {
			finalAnswer = text.String()
		}
	}
	return finalAnswer
}

func recoverInterruptedTurns(
	store *meta.AgentTurnStore,
	stateDir string,
) (failed, cancelled, reconciled int, err error) {
	turns, err := store.Outstanding()
	if err != nil {
		return 0, 0, 0, err
	}
	for _, turn := range turns {
		if turn.Status == meta.AgentTurnQueued {
			if _, err := store.Transition(turn.ID, meta.AgentTurnTransition{
				Status:         meta.AgentTurnCancelled,
				ErrorCode:      "backend_restarted",
				ErrorText:      "Backend restarted before this queued Turn was dispatched.",
				StopReason:     "backend_restarted",
				TerminalSource: "recovery_policy",
			}); err != nil {
				return failed, cancelled, reconciled, err
			}
			cancelled++
			continue
		}

		record, loadErr := loadRuntimeSessionRecord(stateDir, turn.SessionID)
		snapshot, ok := record.Acpx.TurnResults[turn.ID]
		snapshotTerminal := snapshot.Status == meta.AgentTurnCompleted ||
			snapshot.Status == meta.AgentTurnFailed ||
			snapshot.Status == meta.AgentTurnCancelled
		if loadErr == nil && ok && snapshotTerminal {
			change := meta.AgentTurnTransition{
				Status:           snapshot.Status,
				FinalAnswer:      finalAnswerForTurn(record.Messages, turn.ID),
				RuntimeRecordID:  turn.SessionID,
				RuntimeRequestID: turn.ID,
				PromptMessageID:  snapshot.PromptMessageID,
				StopReason:       snapshot.StopReason,
				TerminalSource:   "reconciled_runtime_record",
				LastEventSeq:     snapshot.LastEventSeq,
				ErrorCode:        snapshot.ErrorCode,
			}
			if _, err := store.Transition(turn.ID, change); err != nil {
				return failed, cancelled, reconciled, err
			}
			reconciled++
			continue
		}

		errorCode := "runtime_state_unknown"
		if loadErr == nil && ok && snapshot.Status == meta.AgentTurnRunning {
			errorCode = "backend_restarted"
		}
		if _, err := store.Transition(turn.ID, meta.AgentTurnTransition{
			Status:         meta.AgentTurnFailed,
			ErrorCode:      errorCode,
			ErrorText:      fmt.Sprintf("Backend restarted with no durable runtime terminal for Turn %s.", turn.ID),
			StopReason:     "backend_restarted",
			TerminalSource: "recovery_policy",
		}); err != nil {
			return failed, cancelled, reconciled, err
		}
		failed++
	}
	return failed, cancelled, reconciled, nil
}
