package speechclip

import (
	"encoding/json"
	"os"
	"testing"
)

// validTimelineFixture is the canonical v1 timelines/main.json example.
// Derived from the smoke-test transcript (highlights_test.go):
//
//	i=0 start=0    end=3000  corrected_text="今天是一个硬件的采购记录。"
//	i=2 start=6000 end=9000  corrected_text="芯片是ESP32，不支持经典蓝牙。"
var validTimelineFixture = &Timeline{
	Version: 1,
	ID:      "main",
	AssetID: "a01",
	Clips: []TimelineClip{
		{
			StartMs:           0,
			EndMs:             3000,
			Text:              "今天是一个硬件的采购记录。",
			SourceSentenceIDs: []int{0},
		},
		{
			StartMs:           6000,
			EndMs:             9000,
			Text:              "芯片是ESP32，不支持经典蓝牙。",
			SourceSentenceIDs: []int{2},
		},
	},
}

func TestValidateTimeline_Valid(t *testing.T) {
	if err := ValidateTimeline(validTimelineFixture); err != nil {
		t.Fatalf("expected valid fixture to pass: %v", err)
	}
}

func TestValidateTimeline_WithDurationMs(t *testing.T) {
	d := 10000
	tl := *validTimelineFixture
	tl.DurationMs = &d
	if err := ValidateTimeline(&tl); err != nil {
		t.Fatalf("expected valid with durationMs=10000: %v", err)
	}
}

// invalidTimestampFixture: startMs equals endMs — violates 0 <= startMs < endMs.
func TestValidateTimeline_InvalidTimestamp(t *testing.T) {
	tl := &Timeline{
		Version: 1,
		ID:      "main",
		AssetID: "a01",
		Clips: []TimelineClip{
			{StartMs: 3000, EndMs: 3000, Text: "bad", SourceSentenceIDs: []int{1}},
		},
	}
	if err := ValidateTimeline(tl); err == nil {
		t.Fatal("expected error for startMs == endMs, got nil")
	}
}

func TestValidateTimeline_StartMsNegative(t *testing.T) {
	tl := &Timeline{
		Version: 1,
		ID:      "main",
		AssetID: "a01",
		Clips: []TimelineClip{
			{StartMs: -1, EndMs: 3000, Text: "neg", SourceSentenceIDs: []int{0}},
		},
	}
	if err := ValidateTimeline(tl); err == nil {
		t.Fatal("expected error for startMs < 0")
	}
}

func TestValidateTimeline_WrongVersion(t *testing.T) {
	tl := *validTimelineFixture
	tl.Version = 2
	if err := ValidateTimeline(&tl); err == nil {
		t.Fatal("expected error for version != 1")
	}
}

func TestValidateTimeline_WrongID(t *testing.T) {
	tl := *validTimelineFixture
	tl.ID = "other"
	if err := ValidateTimeline(&tl); err == nil {
		t.Fatal("expected error for id != \"main\"")
	}
}

func TestValidateTimeline_EmptyAssetID(t *testing.T) {
	tl := *validTimelineFixture
	tl.AssetID = ""
	if err := ValidateTimeline(&tl); err == nil {
		t.Fatal("expected error for empty assetId")
	}
}

func TestValidateTimeline_EmptyClips(t *testing.T) {
	tl := *validTimelineFixture
	tl.Clips = nil
	if err := ValidateTimeline(&tl); err == nil {
		t.Fatal("expected error for empty clips")
	}
}

func TestValidateTimeline_DurationMsTooSmall(t *testing.T) {
	d := 8000 // last clip endMs is 9000
	tl := *validTimelineFixture
	tl.DurationMs = &d
	if err := ValidateTimeline(&tl); err == nil {
		t.Fatal("expected error for durationMs < last clip endMs")
	}
}

func TestSaveLoadTimeline(t *testing.T) {
	ws := t.TempDir()
	if err := saveTimeline(ws, validTimelineFixture); err != nil {
		t.Fatalf("saveTimeline: %v", err)
	}
	got, err := loadTimeline(ws)
	if err != nil {
		t.Fatalf("loadTimeline: %v", err)
	}
	a, _ := json.Marshal(validTimelineFixture)
	b, _ := json.Marshal(got)
	if string(a) != string(b) {
		t.Fatalf("round-trip mismatch:\nwant %s\n got %s", a, b)
	}
}

// TestValidTimelineFixtureJSON verifies the fixture serialises to the
// expected timelines/main.json shape.
func TestValidTimelineFixtureJSON(t *testing.T) {
	data, _ := json.MarshalIndent(validTimelineFixture, "", "  ")
	var check map[string]any
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"version", "id", "assetId", "clips"} {
		if _, ok := check[key]; !ok {
			t.Errorf("fixture JSON missing key %q", key)
		}
	}
	// Verify no unexpected sentinel values survive
	if _, hasD := check["durationMs"]; hasD {
		t.Error("fixture should not include durationMs when nil")
	}
}

// TestTimelineInvalidTimestampFixtureJSON documents the invalid-timestamp
// shape — kept as a named fixture for the frontend fixture parity.
func TestTimelineInvalidTimestampFixtureJSON(t *testing.T) {
	fixture := &Timeline{
		Version: 1,
		ID:      "main",
		AssetID: "a01",
		Clips: []TimelineClip{
			{StartMs: 5000, EndMs: 3000, Text: "reversed", SourceSentenceIDs: []int{0}},
		},
	}
	if err := ValidateTimeline(fixture); err == nil {
		t.Fatal("invalid-timestamp fixture must fail validation")
	}
	_ = os.WriteFile(os.DevNull, func() []byte { b, _ := json.MarshalIndent(fixture, "", "  "); return b }(), 0o644)
}
