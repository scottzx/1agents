package media

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/domainstore"
)

// RegisterRoutes wires the media app's HTTP handlers onto mux. The orchestrator
// calls this one line from the central router (it does not need to know the
// individual routes). All routes are under /api/apps/media/.
//
//	media.RegisterRoutes(mux)
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/apps/media/projects", handleProjects)          // GET list / POST create
	mux.HandleFunc("/api/apps/media/materials", handleMaterials)        // GET list?projectId / POST add(json)
	mux.HandleFunc("/api/apps/media/materials/upload", handleUpload)    // POST multipart → bytes on file face
	mux.HandleFunc("/api/apps/media/pipeline", handlePipeline)          // POST {projectId,materialId} → launch
	mux.HandleFunc("/api/apps/media/segments", handleSegments)          // GET list?materialId / POST decision
	mux.HandleFunc("/api/apps/media/tasks", handleBusinessTasks)        // GET ?ref= → inline task state
	mux.HandleFunc("/api/apps/media/human/resolve", handleHumanResolve) // POST resolve human gate
	mux.HandleFunc("/api/apps/media/retrim", handleRetrim)              // POST → trim function task
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// ── projects ────────────────────────────────────────────────────────────────

func handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := ListProjects(r.URL.Query().Get("workspace"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
	case http.MethodPost:
		var body struct {
			ProjectID string `json:"projectId"`
			Workspace string `json:"workspace"`
			Title     string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if body.Workspace == "" || body.Title == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("workspace and title are required"))
			return
		}
		cp, err := CreateProject(body.ProjectID, body.Workspace, body.Title)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, cp)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── materials ───────────────────────────────────────────────────────────────

func handleMaterials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projectID := r.URL.Query().Get("projectId")
		if projectID == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("projectId is required"))
			return
		}
		mats, err := ListMaterials(projectID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"materials": mats})
	case http.MethodPost:
		var body Material
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if body.ProjectID == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("projectId is required"))
			return
		}
		m, err := AddMaterial(body)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleUpload lands the uploaded/recorded bytes under the project workspace via
// the file face (domainstore.ArtifactDir), then stores only the path + metadata
// in media_material (#336: bytes on disk, path in domain table).
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil { // 64 MiB in memory, rest to temp
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	projectID := r.FormValue("projectId")
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("projectId is required"))
		return
	}
	cp, ok, err := GetProject(projectID)
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("content project not found"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("file field required: %w", err))
		return
	}
	defer file.Close()

	dir, err := domainstore.ArtifactDir(cp.Workspace, AppID, "素材库")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dest := filepath.Join(dir, safeName(header.Filename))
	out, err := os.Create(dest)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out.Close()

	rel, _ := domainstore.RelativePath(cp.Workspace, dest)
	dur, _ := strconv.ParseFloat(r.FormValue("duration"), 64)
	kind := r.FormValue("kind")
	if kind == "" {
		kind = guessKind(header.Filename)
	}
	m, err := AddMaterial(Material{
		ProjectID: projectID,
		Kind:      kind,
		FilePath:  rel,
		Duration:  dur,
		Metadata:  map[string]string{"originalName": header.Filename},
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// ── pipeline / segments / tasks / human gate ─────────────────────────────────

func handlePipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ProjectID  string `json:"projectId"`
		MaterialID string `json:"materialId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ids, err := LaunchProcessingPipeline(body.ProjectID, body.MaterialID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"taskIds": ids})
}

func handleSegments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		materialID := r.URL.Query().Get("materialId")
		if materialID == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("materialId is required"))
			return
		}
		segs, err := ListSegments(materialID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"segments": segs})
	case http.MethodPost:
		var body struct {
			SegmentID string `json:"segmentId"`
			Decision  string `json:"decision"` // keep | drop | undecided
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if body.SegmentID == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("segmentId is required"))
			return
		}
		if err := SetSegmentDecision(body.SegmentID, body.Decision); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleBusinessTasks returns the inline task state for a business_ref so the
// 阶段追踪 UI can show per-material pipeline state (reverse binding seam).
func handleBusinessTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("ref is required"))
		return
	}
	a, _, err := runtime()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	tasks, err := a.ListTasksForBusiness(ref)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func handleHumanResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		TaskID  string         `json:"taskId"`
		Verdict string         `json:"verdict"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.TaskID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("taskId is required"))
		return
	}
	if err := ResolveHumanTask(body.TaskID, body.Verdict, body.Payload); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleRetrim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ProjectID  string  `json:"projectId"`
		MaterialID string  `json:"materialId"`
		In         string  `json:"in"`
		Out        string  `json:"out"`
		Start      float64 `json:"start"`
		End        float64 `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	id, err := LaunchRetrimTask(body.ProjectID, body.MaterialID, body.In, body.Out, body.Start, body.End)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"taskId": id})
}

// ── helpers ──────────────────────────────────────────────────────────────────

// safeName strips directory components and path separators from an upload name.
func safeName(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "..", "_")
	if name == "" || name == "." || name == "/" {
		return "upload.bin"
	}
	return name
}

func guessKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp3", ".wav", ".m4a", ".aac", ".flac":
		return "audio"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return "image"
	default:
		return "video"
	}
}
