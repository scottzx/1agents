package speechclip

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

//go:embed scripts/transcribe.py
var transcribePy []byte

func init() {
	// One registration, two consumptions (standalone function task + agent tool).
	taskapi.RegisterFunction("speech_clip.transcribe", runTranscribe)
}

// runTranscribe is the executor=function handler. It shells out to FunClip's venv
// to transcribe one asset, writing transcripts/<assetId>.jsonl (one sentence per
// line, tagged with the source asset), and returns a JSON summary that the
// function runner persists to task.Result.
func runTranscribe(ctx taskapi.FunctionContext) (any, error) {
	ws := ctx.Task.WorkspacePath
	assetID := assetIDFromRef(ctx.Task.BusinessRef)
	if ws == "" || assetID == "" {
		return nil, fmt.Errorf("speech_clip.transcribe: missing workspace(%q) or asset ref(%q)", ws, ctx.Task.BusinessRef)
	}

	base := appDir(ws)
	audio, err := findAsset(base, assetID)
	if err != nil {
		return nil, err
	}
	outJSONL := filepath.Join(base, "transcripts", assetID+".jsonl")

	fdir, err := funclipDir()
	if err != nil {
		return nil, err
	}
	py := filepath.Join(fdir, ".venv", "bin", "python")
	if _, statErr := os.Stat(py); statErr != nil {
		return nil, fmt.Errorf("speech_clip: FunClip venv python not found at %s (run: cd %s && uv venv .venv && uv pip install -r requirements.txt)", py, fdir)
	}
	script, err := materializeScript()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(py, script, audio, outJSONL, assetID, fdir)
	cmd.Env = append(os.Environ(), "MODELSCOPE_CACHE="+modelscopeCache())
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return nil, fmt.Errorf("speech_clip: transcribe failed: %v: %s", err, tail(stderr, 800))
	}

	// The script prints a JSON summary as its last stdout line.
	var summary map[string]any
	if e := json.Unmarshal([]byte(lastLine(out)), &summary); e != nil {
		return map[string]any{"asset": assetID, "raw": lastLine(out)}, nil
	}
	return summary, nil
}

// ── path + parsing helpers ───────────────────────────────────────────────────

// appDir is the per-project data root: <workspace>/.artifacts/speech_clip/
// (matches templateregistry's .artifacts/<AppID>/ scaffold convention).
func appDir(ws string) string {
	return filepath.Join(ws, ".artifacts", AppID)
}

// assetIDFromRef parses "speech_clip:asset:<id>" → "<id>".
func assetIDFromRef(ref string) string {
	const prefix = "speech_clip:asset:"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	return ""
}

// findAsset returns the first file in assets/ whose name (sans extension) is assetID.
func findAsset(base, assetID string) (string, error) {
	matches, _ := filepath.Glob(filepath.Join(base, "assets", assetID+".*"))
	if len(matches) == 0 {
		return "", fmt.Errorf("speech_clip: asset %q not found under %s", assetID, filepath.Join(base, "assets"))
	}
	return matches[0], nil
}

// funclipDir resolves the FunClip checkout. Prefers $FUNCLIP_DIR; falls back to
// modules/FunClip relative to the process working directory (dev daemon runs from
// the repo). Returns an error when neither exists.
func funclipDir() (string, error) {
	if d := os.Getenv("FUNCLIP_DIR"); d != "" {
		return d, nil
	}
	if cwd, err := os.Getwd(); err == nil {
		guess := filepath.Join(cwd, "modules", "FunClip")
		if _, err := os.Stat(guess); err == nil {
			return guess, nil
		}
	}
	return "", fmt.Errorf("speech_clip: FunClip dir not found; set $FUNCLIP_DIR to the FunClip checkout")
}

// modelscopeCache is where FunClip's models live. Defaults to the modelscope
// default (~/.cache/modelscope) where they were already downloaded.
func modelscopeCache() string {
	if c := os.Getenv("MODELSCOPE_CACHE"); c != "" {
		return c
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "modelscope")
}

// materializeScript writes the embedded transcribe.py to a stable cache path and
// returns it (the compiled binary carries no source tree).
func materializeScript() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".1agents", "speech_clip")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "transcribe.py")
	if err := os.WriteFile(p, transcribePy, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
