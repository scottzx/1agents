package speechclip

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Timeline is the v1 timelines/main.json contract for the Content Studio
// Remotion timeline preview. Go validation here is authoritative; the TS
// counterpart in frontend/src/apps/speechclip/timeline.ts is UI-only early
// feedback.
//
// Field alignment with JSONL sources:
//   - Clip.SourceSentenceIDs → transcript `i` field
//   - Clip.StartMs / EndMs   → transcript `start` / `end` (already in ms)
//   - Clip.Text              → highlight `corrected_text` (or raw `text`)
type Timeline struct {
	Version    int            `json:"version"`              // must be 1
	ID         string         `json:"id"`                   // must be "main"
	AssetID    string         `json:"assetId"`              // root asset id, non-empty
	DurationMs *int           `json:"durationMs,omitempty"` // optional; must be >= last clip endMs
	Clips      []TimelineClip `json:"clips"`                // non-empty ordered edit segments
}

// TimelineClip is one edit segment. SourceSentenceIDs references the `i` field
// from transcripts/<assetId>.jsonl; Text is typically corrected_text from
// highlights/<assetId>.jsonl.
type TimelineClip struct {
	StartMs           int    `json:"startMs"`           // ms, 0 <= startMs < endMs
	EndMs             int    `json:"endMs"`             // ms
	Text              string `json:"text"`              // display text (corrected_text or raw)
	SourceSentenceIDs []int  `json:"sourceSentenceIds"` // sentence `i` values this clip covers
}

// ValidateTimeline enforces the authoritative v1 contract constraints.
func ValidateTimeline(t *Timeline) error {
	if t.Version != 1 {
		return fmt.Errorf("version must be 1, got %d", t.Version)
	}
	if t.ID != "main" {
		return fmt.Errorf("id must be \"main\", got %q", t.ID)
	}
	if t.AssetID == "" {
		return errors.New("assetId is required")
	}
	if len(t.Clips) == 0 {
		return errors.New("clips must be non-empty")
	}
	for idx, c := range t.Clips {
		if c.StartMs < 0 {
			return fmt.Errorf("clip[%d]: startMs must be >= 0, got %d", idx, c.StartMs)
		}
		if c.StartMs >= c.EndMs {
			return fmt.Errorf("clip[%d]: startMs (%d) must be < endMs (%d)", idx, c.StartMs, c.EndMs)
		}
	}
	if t.DurationMs != nil {
		d := *t.DurationMs
		if d <= 0 {
			return fmt.Errorf("durationMs must be > 0, got %d", d)
		}
		last := t.Clips[len(t.Clips)-1]
		if d < last.EndMs {
			return fmt.Errorf("durationMs (%d) must be >= last clip endMs (%d)", d, last.EndMs)
		}
	}
	return nil
}

// loadTimeline reads and validates timelines/main.json for a workspace.
func loadTimeline(ws string) (*Timeline, error) {
	data, err := os.ReadFile(filepath.Join(appDir(ws), "timelines", "main.json"))
	if err != nil {
		return nil, err
	}
	var t Timeline
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	if err := ValidateTimeline(&t); err != nil {
		return nil, fmt.Errorf("invalid timeline: %w", err)
	}
	return &t, nil
}

// saveTimeline validates and writes timelines/main.json.
func saveTimeline(ws string, t *Timeline) error {
	if err := ValidateTimeline(t); err != nil {
		return fmt.Errorf("invalid timeline: %w", err)
	}
	dir := filepath.Join(appDir(ws), "timelines")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(t, "", "  ")
	return os.WriteFile(filepath.Join(dir, "main.json"), data, 0o644)
}
