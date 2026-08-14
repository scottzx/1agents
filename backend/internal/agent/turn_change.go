package agent

import (
	"encoding/json"
	"log"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func (c *AcpxClient) ingestTurnChangeReports(bridge *ActiveBridge, raw []byte) {
	if c == nil || c.turnStore == nil || bridge == nil {
		return
	}
	store := c.turnStore.ChangeReports()
	if store == nil {
		return
	}

	var payload struct {
		Items []meta.HistoryChangeItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		log.Printf("[acpx_client] turn change peek failed for session %s: %v", bridge.SessionID, err)
		return
	}

	bridge.mu.Lock()
	lastFinished := bridge.lastFinishedTurnID
	bridge.mu.Unlock()

	aggregated := meta.AggregateTurnChanges(payload.Items)
	if len(aggregated) == 0 && lastFinished == "" {
		return
	}

	resolvedLastFinished := lastFinished
	if lastFinished != "" {
		if id, ok, err := store.ResolveTurnID(bridge.SessionID, lastFinished); err == nil && ok {
			resolvedLastFinished = id
		}
	}

	now := time.Now().UTC()
	processedFinished := false
	for historyTurnID, files := range aggregated {
		canonicalID, ok, err := store.ResolveTurnID(bridge.SessionID, historyTurnID)
		if err != nil {
			log.Printf("[acpx_client] resolve turn %s for change report: %v", historyTurnID, err)
			continue
		}
		if !ok {
			continue
		}
		if canonicalID == resolvedLastFinished {
			processedFinished = true
		}
		existing, _, err := store.Get(canonicalID)
		if err != nil {
			log.Printf("[acpx_client] get change report %s: %v", canonicalID, err)
			continue
		}
		var existingPtr *meta.TurnChangeReport
		if existing.TurnID != "" {
			existingPtr = &existing
		}
		if !meta.NeedsRecompute(existingPtr, meta.TurnChangeRecipeVersion, resolvedLastFinished) {
			continue
		}
		added, deleted, modified := meta.CountTurnChangeOps(files)
		source := meta.TurnChangeBackfill
		if canonicalID == resolvedLastFinished {
			source = meta.TurnChangeLive
		}
		if len(files) == 0 {
			source = meta.TurnChangeUnavailable
		}
		if err := store.Upsert(meta.TurnChangeReport{
			TurnID:        canonicalID,
			RecipeVersion: meta.TurnChangeRecipeVersion,
			AddedCount:    added,
			DeletedCount:  deleted,
			ModifiedCount: modified,
			Files:         files,
			Source:        source,
			ComputedAt:    now,
		}); err != nil {
			log.Printf("[acpx_client] upsert change report %s: %v", canonicalID, err)
		}
	}

	if processedFinished {
		bridge.mu.Lock()
		if bridge.lastFinishedTurnID == lastFinished {
			bridge.lastFinishedTurnID = ""
		}
		bridge.mu.Unlock()
	}
}

func rememberFinishedTurn(bridge *ActiveBridge, turnID string) {
	if bridge == nil || turnID == "" {
		return
	}
	bridge.mu.Lock()
	bridge.lastFinishedTurnID = turnID
	bridge.mu.Unlock()
}
