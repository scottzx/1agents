package speechclip

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// Handler serves the speech_clip HTTP surface. It dispatches heavy work through
// the task kernel (taskAPI) rather than doing it inline, so every step is a
// visible task. Wire it in server.go with NewHandler(taskAPI).
type Handler struct {
	api *taskapi.API
	mu  sync.Mutex // guards project.json read-modify-write (single-user)
}

func NewHandler(api *taskapi.API) *Handler { return &Handler{api: api} }

// ── project.json model ───────────────────────────────────────────────────────

type Asset struct {
	ID      string `json:"id"`
	File    string `json:"file"`  // basename under assets/
	Label   string `json:"label"` // human tag: 个人介绍 / 产品演示 / ...
	AddedAt string `json:"added_at"`
}

type Project struct {
	Assets    []Asset  `json:"assets"`
	Mainline  []string `json:"mainline"` // 主线意图顺序(asset id)
	Stage     string   `json:"stage"`    // imported|transcribed|highlighted|arranged|...
	UpdatedAt string   `json:"updated_at"`
}

func (h *Handler) projectPath(ws string) string {
	return filepath.Join(appDir(ws), "project.json")
}

func (h *Handler) loadProject(ws string) (*Project, error) {
	p := &Project{Stage: "imported"}
	data, err := os.ReadFile(h.projectPath(ws))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (h *Handler) saveProject(ws string, p *Project) error {
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := os.MkdirAll(appDir(ws), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(h.projectPath(ws), data, 0o644)
}

// ── routes ───────────────────────────────────────────────────────────────────

// HandleAssets: POST /api/speech_clip/assets
// body {workspacePath, sourcePath, label} → copies the file into assets/ and
// registers it in project.json. Returns {assetId}.
func (h *Handler) HandleAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspacePath string `json:"workspacePath"`
		SourcePath    string `json:"sourcePath"`
		Label         string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.WorkspacePath == "" || req.SourcePath == "" {
		http.Error(w, "workspacePath and sourcePath required", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	proj, err := h.loadProject(req.WorkspacePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	assetID := fmt.Sprintf("a%02d", len(proj.Assets)+1)
	ext := filepath.Ext(req.SourcePath)
	dstDir := filepath.Join(appDir(req.WorkspacePath), "assets")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dst := filepath.Join(dstDir, assetID+ext)
	if err := copyFile(req.SourcePath, dst); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	proj.Assets = append(proj.Assets, Asset{
		ID:      assetID,
		File:    assetID + ext,
		Label:   req.Label,
		AddedAt: time.Now().UTC().Format(time.RFC3339),
	})
	proj.Mainline = append(proj.Mainline, assetID)
	if err := h.saveProject(req.WorkspacePath, proj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"assetId": assetID, "file": assetID + ext})
}

// HandleUpload: POST /api/speech_clip/assets/upload
// Registers an in-browser recording as an asset. Two shapes:
//   - {videoBase64, audioBase64}: separate screen-video + audio tracks are muxed
//     into one playable webm (video retained for 混剪; transcription extracts its
//     audio).
//   - {dataBase64}: a single blob (audio-only fallback), written as-is.
//
// body: {workspacePath, label, ext, videoBase64?, audioBase64?, dataBase64?}. Returns {assetId}.
func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspacePath string `json:"workspacePath"`
		Label         string `json:"label"`
		Ext           string `json:"ext"`
		VideoBase64   string `json:"videoBase64"`
		AudioBase64   string `json:"audioBase64"`
		DataBase64    string `json:"dataBase64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.WorkspacePath == "" {
		http.Error(w, "workspacePath required", http.StatusBadRequest)
		return
	}
	if req.VideoBase64 == "" && req.AudioBase64 == "" && req.DataBase64 == "" {
		http.Error(w, "one of videoBase64/audioBase64/dataBase64 required", http.StatusBadRequest)
		return
	}
	ext := req.Ext
	if ext == "" {
		ext = ".webm"
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	proj, err := h.loadProject(req.WorkspacePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	assetID := fmt.Sprintf("a%02d", len(proj.Assets)+1)
	dstDir := filepath.Join(appDir(req.WorkspacePath), "assets")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	finalPath := filepath.Join(dstDir, assetID+ext)

	if req.VideoBase64 != "" && req.AudioBase64 != "" {
		// Mux screen video + audio into the asset file.
		vTmp := finalPath + ".v.tmp"
		aTmp := finalPath + ".a.tmp"
		if err := writeBase64(vTmp, req.VideoBase64); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := writeBase64(aTmp, req.AudioBase64); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer os.Remove(vTmp)
		defer os.Remove(aTmp)
		if err := muxAV(vTmp, aTmp, finalPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		blob := req.DataBase64
		if blob == "" {
			blob = req.AudioBase64
		}
		if blob == "" {
			blob = req.VideoBase64
		}
		if err := writeBase64(finalPath, blob); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	proj.Assets = append(proj.Assets, Asset{
		ID:      assetID,
		File:    assetID + ext,
		Label:   req.Label,
		AddedAt: time.Now().UTC().Format(time.RFC3339),
	})
	proj.Mainline = append(proj.Mainline, assetID)
	if err := h.saveProject(req.WorkspacePath, proj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"assetId": assetID, "file": assetID + ext})
}

// writeBase64 decodes a base64 string and writes it to path.
func writeBase64(path, b64 string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("bad base64: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// HandleTranscribe: POST /api/speech_clip/transcribe
// body {workspacePath, assetId} → dispatches an executor=function task. Returns {taskId}.
func (h *Handler) HandleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspacePath string `json:"workspacePath"`
		AssetID       string `json:"assetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.WorkspacePath == "" || req.AssetID == "" {
		http.Error(w, "workspacePath and assetId required", http.StatusBadRequest)
		return
	}
	taskID, err := h.api.DispatchTask(AppID, taskapi.DispatchSpec{
		Title:         "转录 " + req.AssetID,
		Description:   "FunClip 语音识别：素材 " + req.AssetID + " → transcripts/" + req.AssetID + ".jsonl（逐句带来源标记+说话人）。",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  "speech_clip.transcribe",
		BusinessRef:   "speech_clip:asset:" + req.AssetID,
		WorkspacePath: req.WorkspacePath,
		Priority:      "medium",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"taskId": taskID})
}

// HandleHighlights serves /api/speech_clip/highlights:
//   - POST {workspacePath, assetId} → dispatch the 1acp correct+extract task (also
//     fired automatically by the completion-hook chain after transcription).
//   - GET  ?workspacePath=&assetId=  → the graded highlight rows for one asset.
func (h *Handler) HandleHighlights(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			WorkspacePath string `json:"workspacePath"`
			AssetID       string `json:"assetId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.WorkspacePath == "" || req.AssetID == "" {
			http.Error(w, "workspacePath and assetId required", http.StatusBadRequest)
			return
		}
		taskID, err := dispatchHighlight(h.api, req.WorkspacePath, req.AssetID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"taskId": taskID})
	case http.MethodGet:
		ws := r.URL.Query().Get("workspacePath")
		assetID := r.URL.Query().Get("assetId")
		if ws == "" || assetID == "" {
			http.Error(w, "workspacePath and assetId required", http.StatusBadRequest)
			return
		}
		rows := readJSONL(filepath.Join(appDir(ws), "highlights", assetID+".jsonl"))
		writeJSON(w, map[string]any{"asset": assetID, "highlights": rows})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandlePick: POST /api/speech_clip/pick {workspacePath, assetId, i, picked}
// toggles the picked flag on one highlight row and rewrites the jsonl. This is
// the human金句-selection persisted back over the 1acp suggestion.
func (h *Handler) HandlePick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspacePath string `json:"workspacePath"`
		AssetID       string `json:"assetId"`
		I             int    `json:"i"`
		Picked        bool   `json:"picked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.WorkspacePath == "" || req.AssetID == "" {
		http.Error(w, "workspacePath and assetId required", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	path := filepath.Join(appDir(req.WorkspacePath), "highlights", req.AssetID+".jsonl")
	rows := readJSONL(path)
	if len(rows) == 0 {
		http.Error(w, "no highlights for asset", http.StatusNotFound)
		return
	}
	found := false
	for _, row := range rows {
		if int(toFloat(row["i"])) == req.I {
			row["picked"] = req.Picked
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "sentence index not found", http.StatusNotFound)
		return
	}
	var buf []byte
	for _, row := range rows {
		line, _ := json.Marshal(row)
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "i": req.I, "picked": req.Picked})
}

// HandleProject: GET /api/speech_clip/project?workspacePath=
// returns project.json enriched with per-asset transcript status.
func (h *Handler) HandleProject(w http.ResponseWriter, r *http.Request) {
	ws := r.URL.Query().Get("workspacePath")
	if ws == "" {
		http.Error(w, "workspacePath required", http.StatusBadRequest)
		return
	}
	proj, err := h.loadProject(ws)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type assetView struct {
		Asset
		TranscriptSentences int  `json:"transcriptSentences"`
		Transcribed         bool `json:"transcribed"`
		Highlights          int  `json:"highlights"`
		Highlighted         bool `json:"highlighted"`
	}
	views := make([]assetView, 0, len(proj.Assets))
	for _, a := range proj.Assets {
		n := countJSONL(filepath.Join(appDir(ws), "transcripts", a.ID+".jsonl"))
		hn := countJSONL(filepath.Join(appDir(ws), "highlights", a.ID+".jsonl"))
		views = append(views, assetView{
			Asset: a, TranscriptSentences: n, Transcribed: n > 0,
			Highlights: hn, Highlighted: hn > 0,
		})
	}
	writeJSON(w, map[string]any{
		"stage":    proj.Stage,
		"mainline": proj.Mainline,
		"assets":   views,
	})
}

// HandleTranscript: GET /api/speech_clip/transcript?workspacePath=&assetId=
// returns the raw sentence rows for one asset (for the table view).
func (h *Handler) HandleTranscript(w http.ResponseWriter, r *http.Request) {
	ws := r.URL.Query().Get("workspacePath")
	assetID := r.URL.Query().Get("assetId")
	if ws == "" || assetID == "" {
		http.Error(w, "workspacePath and assetId required", http.StatusBadRequest)
		return
	}
	rows := readJSONL(filepath.Join(appDir(ws), "transcripts", assetID+".jsonl"))
	writeJSON(w, map[string]any{"asset": assetID, "sentences": rows})
}

// ── small helpers ────────────────────────────────────────────────────────────

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func countJSONL(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range splitLines(string(data)) {
		if line != "" {
			n++
		}
	}
	return n
}

func readJSONL(path string) []map[string]any {
	out := []map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return toFloat(out[i]["i"]) < toFloat(out[j]["i"])
	})
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func toFloat(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
