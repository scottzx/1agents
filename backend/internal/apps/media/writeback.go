package media

import (
	"encoding/json"
	"log"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// onTaskCompletion is the domain writeback hook (§反向写回). Registered via
// RegisterCompletionHook; fired when ANY task reaches a terminal state. We only
// act on media-owned tasks (business_ref prefix "media:"), so other apps' tasks
// pass through untouched.
func onTaskCompletion(ev taskapi.CompletionEvent) {
	if ev.Status != meta.TaskStatusCompleted {
		return
	}
	a, _, err := runtime()
	if err != nil {
		return
	}
	task, ok, err := a.QueryTask(ev.TaskID)
	if err != nil || !ok {
		return
	}
	entity, id, ok := parseBusinessRef(task.BusinessRef)
	if !ok || entity != "material" {
		return // not a media-material task
	}
	materialID := id

	switch task.Milestone {
	case "silence_detect":
		// Persist the computed segments so the 段落取舍 UI can show keep/drop.
		writeSilenceSegments(materialID, ev.Result)
		_ = SetMaterialStage(materialID, "silence_detected")
	case "trim":
		_ = SetMaterialStage(materialID, "trimmed")
	case "approve":
		// Human gate cleared → mark approved (the UI refresh picks this up).
		_ = SetMaterialStage(materialID, "approved")
	}
}

// writeSilenceSegments parses a silence_detect result JSON and stores its
// segments (decision=undecided) for the material.
func writeSilenceSegments(materialID, resultJSON string) {
	var res silenceDetectResult
	if err := json.Unmarshal([]byte(resultJSON), &res); err != nil {
		log.Printf("[media] writeback: parse silence result for %s: %v", materialID, err)
		return
	}
	segs := make([]Segment, 0, len(res.Segments))
	for _, s := range res.Segments {
		segs = append(segs, Segment{
			MaterialID: materialID,
			Start:      s.Start,
			End:        s.End,
			Decision:   "undecided",
		})
	}
	if err := ReplaceSegments(materialID, segs); err != nil {
		log.Printf("[media] writeback: store segments for %s: %v", materialID, err)
	}
}
