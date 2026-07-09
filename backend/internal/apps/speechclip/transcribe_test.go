package speechclip

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// TestRunTranscribeSmoke exercises the full function-handler path (asset lookup →
// FunClip subprocess → jsonl write → summary parse). Slow (loads models); gated
// behind env so it never runs in normal CI.
//
//	FUNCLIP_DIR=.../modules/FunClip \
//	SPEECH_CLIP_TEST_AUDIO=.../audio.mp3 \
//	go test ./internal/apps/speechclip/ -run TestRunTranscribeSmoke -v -timeout 600s
func TestRunTranscribeSmoke(t *testing.T) {
	if os.Getenv("FUNCLIP_DIR") == "" {
		t.Skip("set FUNCLIP_DIR to run")
	}
	src := os.Getenv("SPEECH_CLIP_TEST_AUDIO")
	if src == "" {
		t.Skip("set SPEECH_CLIP_TEST_AUDIO to run")
	}

	ws := t.TempDir()
	assetsDir := filepath.Join(appDir(ws), "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, filepath.Join(assetsDir, "a01"+filepath.Ext(src))); err != nil {
		t.Fatal(err)
	}

	res, err := runTranscribe(taskapi.FunctionContext{Task: meta.Task{
		WorkspacePath: ws,
		BusinessRef:   "speech_clip:asset:a01",
	}})
	if err != nil {
		t.Fatalf("runTranscribe: %v", err)
	}

	summary, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %T", res)
	}
	if n, _ := summary["sentences"].(float64); n <= 0 {
		t.Fatalf("expected sentences>0, got %v", summary["sentences"])
	}

	jsonl := filepath.Join(appDir(ws), "transcripts", "a01.jsonl")
	if countJSONL(jsonl) == 0 {
		t.Fatalf("transcript jsonl empty: %s", jsonl)
	}
	rows := readJSONL(jsonl)
	if rows[0]["asset"] != "a01" {
		t.Fatalf("first row missing asset tag: %v", rows[0])
	}
	t.Logf("OK: %d sentences, first=%q spk=%v", len(rows), rows[0]["text"], rows[0]["spk"])
}
