package execution

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Handler exposes the public ExecutionJob API. The caller mounts Root at
// /api/execution-jobs and Item at /api/execution-jobs/.
type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Root(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs, err := h.service.ListJobs(r.URL.Query().Get("projectId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": jobs})
	case http.MethodPost:
		var input CreateJobInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		job, err := h.service.CreateJob(input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, job)
	default:
		methodNotAllowed(w)
	}
}

func (h *Handler) Item(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/execution-jobs/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			job, err := h.service.GetJob(id)
			if err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
		case http.MethodPut:
			var patch UpdateJobInput
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			job, err := h.service.UpdateJob(id, patch)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
		default:
			methodNotAllowed(w)
		}
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "pause":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		statusAction(w, h.service.PauseJob(id))
	case "resume":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		statusAction(w, h.service.ResumeJob(id))
	case "archive":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		statusAction(w, h.service.ArchiveJob(id))
	case "trigger":
		switch r.Method {
		case http.MethodPut:
			var spec TriggerSpec
			if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			trigger, err := h.service.UpsertTrigger(id, spec)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, trigger)
		case http.MethodDelete:
			if err := h.service.DeleteTrigger(id); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w)
		}
	case "runs":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		runs, err := h.service.ListRuns(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": runs})
	case "run":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if err := h.service.RunNow(r.Context(), id); err != nil {
			if errors.Is(err, ErrWorkspaceBusy) {
				payload := map[string]any{"accepted": true, "delayed": true}
				var deferred deferredBusyError
				if errors.As(err, &deferred) {
					payload["nextRunAt"] = deferred.next
				}
				writeJSON(w, http.StatusAccepted, payload)
				return
			}
			writeError(w, http.StatusConflict, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
	default:
		http.NotFound(w, r)
	}
}

type errDispatchNotEnabled struct{}

func (errDispatchNotEnabled) Error() string {
	return "execution: dispatch is not enabled until the scheduler migration"
}
func statusAction(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func methodNotAllowed(w http.ResponseWriter) { w.WriteHeader(http.StatusMethodNotAllowed) }
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
