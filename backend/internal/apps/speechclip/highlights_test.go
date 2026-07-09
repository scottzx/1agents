package speechclip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// TestRunHighlightsSmoke drives the real 1acp (acpx→claude) correction + grading
// over a synthetic transcript carrying known ASR errors, and asserts the tech
// terms get corrected. Gated behind ONEACP_DIR (and costs tokens); skipped in CI.
//
//	ONEACP_DIR=.../modules/1acp \
//	go test ./internal/apps/speechclip/ -run TestRunHighlightsSmoke -v -timeout 400s
func TestRunHighlightsSmoke(t *testing.T) {
	if os.Getenv("ONEACP_DIR") == "" {
		t.Skip("set ONEACP_DIR to run")
	}

	ws := t.TempDir()
	tdir := filepath.Join(appDir(ws), "transcripts")
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{
		`{"i":0,"asset":"a01","text":"今天是一个硬件的采根记录。","start":0,"end":3000,"spk":1}`,
		`{"i":1,"asset":"a01","text":"我们买了n五stack的两个设备。","start":3000,"end":6000,"spk":1}`,
		`{"i":2,"asset":"a01","text":"芯片是ESP三二s三，不支持经典蓝牙。","start":6000,"end":9000,"spk":1}`,
		`{"i":3,"asset":"a01","text":"呃，好，评论区见，拜拜。","start":9000,"end":11000,"spk":1}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tdir, "a01.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := runHighlights(taskapi.FunctionContext{Task: meta.Task{
		WorkspacePath: ws,
		BusinessRef:   "speech_clip:highlight:a01",
	}})
	if err != nil {
		t.Fatalf("runHighlights: %v", err)
	}
	t.Logf("summary: %+v", res)

	rows := readJSONL(filepath.Join(appDir(ws), "highlights", "a01.jsonl"))
	if len(rows) == 0 {
		t.Fatal("no highlight rows written")
	}
	// Join all corrected text to assert the corrections landed.
	var all strings.Builder
	for _, r := range rows {
		if r["asset"] != "a01" {
			t.Fatalf("row missing asset tag: %v", r)
		}
		if ct, _ := r["corrected_text"].(string); ct != "" {
			all.WriteString(ct)
			all.WriteString(" ")
		}
	}
	corrected := all.String()
	// Deterministic corrections (English tech terms) must land.
	for _, want := range []string{"M5Stack", "ESP32"} {
		if !strings.Contains(corrected, want) {
			t.Errorf("expected corrected text to contain %q; got: %s", want, corrected)
		}
	}
	// The garbled ASR tokens must be gone (however the model chose to fix them).
	for _, gone := range []string{"n五stack", "ESP三二"} {
		if strings.Contains(corrected, gone) {
			t.Errorf("expected garbled token %q to be corrected away; got: %s", gone, corrected)
		}
	}
}
