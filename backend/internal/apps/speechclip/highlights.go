package speechclip

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/appkit"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

func init() {
	taskapi.RegisterFunction("speech_clip.extract_highlights", runHighlights)
	// Pipeline glue needs the live API (QueryTask + DispatchTask), so it runs in
	// an appkit OnInit rather than a plain init().
	appkit.OnInit(registerPipeline)
}

// registerPipeline wires the completion-hook chain: when a transcribe task
// finishes, auto-dispatch the highlight task for that asset. Function-executor
// tasks fire completion hooks (agent tasks currently do not), so keeping both
// steps as functions makes the chain fire end-to-end.
func registerPipeline(api *taskapi.API) {
	api.RegisterCompletionHook(func(ev taskapi.CompletionEvent) {
		if ev.Status != meta.TaskStatusCompleted {
			return
		}
		task, ok, err := api.QueryTask(ev.TaskID)
		if err != nil || !ok {
			return
		}
		kind, assetID := refKindID(task.BusinessRef)
		if kind != "asset" || assetID == "" {
			return // only chain off a transcribe (speech_clip:asset:<id>) completion
		}
		if _, derr := dispatchHighlight(api, task.WorkspacePath, assetID); derr != nil {
			log.Printf("[speech_clip] auto-dispatch highlight for %s: %v", assetID, derr)
		}
	})
}

// dispatchHighlight enqueues the highlight (correct + extract) function task for
// one asset. Shared by the completion-hook chain and the manual HTTP trigger.
func dispatchHighlight(api *taskapi.API, ws, assetID string) (string, error) {
	return api.DispatchTask(AppID, taskapi.DispatchSpec{
		Title:         "提金句 " + assetID,
		Description:   "1acp 纠错 + 金句提取：素材 " + assetID + " → highlights/" + assetID + ".jsonl。",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  "speech_clip.extract_highlights",
		BusinessRef:   "speech_clip:highlight:" + assetID,
		WorkspacePath: ws,
		Priority:      "medium",
	})
}

// highlightRow is one graded sentence written to highlights/<asset>.jsonl.
type highlightRow struct {
	I             int     `json:"i"`
	Asset         string  `json:"asset"`
	Picked        bool    `json:"picked"`
	Score         float64 `json:"score"`
	Reason        string  `json:"reason"`
	CorrectedText string  `json:"corrected_text"`
}

// runHighlights is the executor=function handler: read the asset transcript,
// send it through 1acp (acpx→claude) for correction + golden-quote grading, and
// write highlights/<asset>.jsonl. Returns a summary for task.Result.
func runHighlights(ctx taskapi.FunctionContext) (any, error) {
	ws := ctx.Task.WorkspacePath
	kind, assetID := refKindID(ctx.Task.BusinessRef)
	if ws == "" || kind != "highlight" || assetID == "" {
		return nil, fmt.Errorf("speech_clip.extract_highlights: bad workspace(%q) or ref(%q)", ws, ctx.Task.BusinessRef)
	}

	base := appDir(ws)
	sentences := readJSONL(filepath.Join(base, "transcripts", assetID+".jsonl"))
	if len(sentences) == 0 {
		return nil, fmt.Errorf("speech_clip: no transcript for asset %q", assetID)
	}

	prompt := buildHighlightPrompt(sentences)
	raw, err := acpxCall(prompt)
	if err != nil {
		return nil, err
	}
	rows, err := parseHighlights(raw, assetID)
	if err != nil {
		return nil, fmt.Errorf("speech_clip: parse 1acp output: %v (raw head: %s)", err, tail(raw, 300))
	}

	outDir := filepath.Join(base, "highlights")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	var buf strings.Builder
	picked := 0
	for _, r := range rows {
		line, _ := json.Marshal(r)
		buf.Write(line)
		buf.WriteByte('\n')
		if r.Picked {
			picked++
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, assetID+".jsonl"), []byte(buf.String()), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"asset":  assetID,
		"graded": len(rows),
		"picked": picked,
		"file":   filepath.Join("highlights", assetID+".jsonl"),
	}, nil
}

// ── 1acp (acpx) invocation ───────────────────────────────────────────────────

func buildHighlightPrompt(sentences []map[string]any) string {
	var b strings.Builder
	b.WriteString("你是中文口播ASR转录的「纠错 + 金句提取」处理器。下面是逐句转录（每句带编号 i）。请：\n")
	b.WriteString("1) 纠正每句里明显的ASR识别错误——错别字、被音译成汉字的英文技术名词（如 ESP32 / M5Stack / Vibe coding / BLE / HFP 等），严格保留原意、口语语气词(呃/嗯/吧)，不要改写句子结构；\n")
	b.WriteString("2) 标注哪些句子是「金句」——适合作为剪辑保留的核心表达(点题/结论/亮点)，picked=true，给 0~1 的 score 和简短 reason；普通过渡句 picked=false；\n")
	b.WriteString("3) 只输出一个 JSON 数组，每个元素 {\"i\":编号, \"picked\":true/false, \"score\":数值, \"reason\":\"\", \"corrected_text\":\"纠错后文本\"}，不要输出任何解释或代码块标记。\n\n")
	b.WriteString("转录：\n")
	for _, s := range sentences {
		i := int(toFloat(s["i"]))
		text, _ := s["text"].(string)
		fmt.Fprintf(&b, "[i=%d] %s\n", i, text)
	}
	return b.String()
}

// acpxCall runs a one-shot acpx→claude prompt with no tools (pure text), quiet
// output. Mirrors the validated CLI path: acpx --format quiet --allowed-tools ""
// --approve-all claude exec -f -.
func acpxCall(prompt string) (string, error) {
	dir, err := oneacpDir()
	if err != nil {
		return "", err
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("speech_clip: node not found on PATH: %w", err)
	}
	cli := filepath.Join(dir, "dist", "cli.js")
	if _, statErr := os.Stat(cli); statErr != nil {
		return "", fmt.Errorf("speech_clip: acpx cli not found at %s", cli)
	}
	cmd := exec.Command(node, cli,
		"--format", "quiet",
		"--allowed-tools", "",
		"--approve-all",
		"--timeout", "300",
		"claude", "exec", "-f", "-",
	)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return "", fmt.Errorf("speech_clip: acpx call failed: %v: %s", err, tail(stderr, 800))
	}
	return strings.TrimSpace(string(out)), nil
}

// oneacpDir resolves the 1acp (acpx) checkout: $ONEACP_DIR, else modules/1acp
// relative to the process working directory.
func oneacpDir() (string, error) {
	if d := os.Getenv("ONEACP_DIR"); d != "" {
		return d, nil
	}
	if cwd, err := os.Getwd(); err == nil {
		guess := filepath.Join(cwd, "modules", "1acp")
		if _, err := os.Stat(guess); err == nil {
			return guess, nil
		}
	}
	return "", fmt.Errorf("speech_clip: 1acp dir not found; set $ONEACP_DIR to the 1acp checkout")
}

// parseHighlights extracts the JSON array from the model output (tolerating a
// stray code fence or prose) and stamps the asset tag onto each row.
func parseHighlights(raw, assetID string) ([]highlightRow, error) {
	body := extractJSONArray(raw)
	var rows []highlightRow
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Asset = assetID
	}
	return rows, nil
}

// extractJSONArray returns the substring from the first '[' to the last ']'.
func extractJSONArray(s string) string {
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// refKindID parses "speech_clip:<kind>:<id>" → (kind, id).
func refKindID(ref string) (string, string) {
	const prefix = "speech_clip:"
	if !strings.HasPrefix(ref, prefix) {
		return "", ""
	}
	rest := strings.TrimPrefix(ref, prefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
