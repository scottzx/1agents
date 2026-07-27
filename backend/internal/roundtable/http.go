package roundtable

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Handler serves /api/roundtable/* endpoints.
//
// Slice 1: rooms + seats.
// Slice 2: R1 chat + brief confirm + turns timeline.
// Slice 3: R2 parallel panelist speeches + referee Summary₂.
// Slice 4: R3 resume + public context + Summary₃ → done.
type Handler struct {
	svc *Service
}

// NewHandler wires HTTP to the service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// NewHandlerDefault opens default stores and returns a Handler with a real
// bridge prompter (ACPX_PORT or 38082).
func NewHandlerDefault() (*Handler, error) {
	return NewHandlerDefaultWithPort(DefaultBridgePort())
}

// NewHandlerDefaultWithPort is used by the server once it knows the 1acp port.
func NewHandlerDefaultWithPort(bridgePort int) (*Handler, error) {
	store, err := NewStore()
	if err != nil {
		return nil, err
	}
	chatStore, err := agent.NewStore()
	if err != nil {
		return nil, err
	}
	if bridgePort <= 0 {
		if v := os.Getenv("ACPX_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil && p > 0 {
				bridgePort = p
			}
		}
	}
	svc := NewService(store, chatStore, NewBridgeSeatPrompter(bridgePort))
	return NewHandler(svc), nil
}

// HandleRoomsRoot serves:
//
//	GET  /api/roundtable/rooms  — list rooms (topic cards; newest first)
//	POST /api/roundtable/rooms  — create room (6 seats, state=drafting_brief)
func (h *Handler) HandleRoomsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listRooms(w, r)
	case http.MethodPost:
		h.createRoom(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleRoomsItem serves:
//
//	GET  /api/roundtable/rooms/{id}         — get room + seats + turns
//	GET  /api/roundtable/rooms/{id}/seats   — list seats
//	GET  /api/roundtable/rooms/{id}/turns   — list turns (main timeline)
//	POST /api/roundtable/rooms/{id}/chat    — R1 user↔referee multi-turn
//	POST /api/roundtable/rooms/{id}/brief/draft   — save user draft
//	POST /api/roundtable/rooms/{id}/brief/propose — agent proposal
//	POST /api/roundtable/rooms/{id}/brief/confirm — user confirms version
//	POST /api/roundtable/rooms/{id}/brief         — legacy management set+confirm
//	POST /api/roundtable/rooms/{id}/r2      — async R2 start; 202 + run_id
//	POST /api/roundtable/rooms/{id}/r3      — async R3 start; 202 + run_id
//	GET  /api/roundtable/rooms/{id}/events  — recoverable events after ?after=<seq>
//	POST /api/roundtable/rooms/{id}/runs/{run}/seats/{role}/retry
//	POST /api/roundtable/rooms/{id}/runs/{run}/skip
//	POST /api/roundtable/rooms/{id}/runs/{run}/summary/retry
//
// Compatibility: old clients may explicitly use r2/r3?wait=1. That opt-in
// path waits and returns the original RunR2Response/RunR3Response with 200.
func (h *Handler) HandleRoomsItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/roundtable/rooms/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.Error(w, "room id required", http.StatusBadRequest)
		return
	}
	id, action, hasAction := strings.Cut(rest, "/")
	if id == "" {
		http.Error(w, "room id required", http.StatusBadRequest)
		return
	}
	if hasAction {
		switch action {
		case "seats":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.listSeats(w, r, id)
		case "turns":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.listTurns(w, r, id)
		case "events":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.listEvents(w, r, id)
		case "chat":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.chat(w, r, id)
		case "brief":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.confirmBrief(w, r, id)
		case "brief/draft":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.saveBriefDraft(w, r, id)
		case "brief/propose":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.proposeBrief(w, r, id)
		case "brief/confirm":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.confirmBriefVersion(w, r, id)
		case "r2":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.runR2(w, r, id)
		case "r3":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.runR3(w, r, id)
		default:
			if strings.HasPrefix(action, "runs/") {
				h.recoverRun(w, r, id, action)
				return
			}
			http.Error(w, "unknown action: "+action, http.StatusNotFound)
		}
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.getRoom(w, r, id)
}

func (h *Handler) recoverRun(w http.ResponseWriter, r *http.Request, roomID, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.Trim(action, "/"), "/")
	if len(parts) < 3 || parts[0] != "runs" || strings.TrimSpace(parts[1]) == "" {
		http.Error(w, "invalid recovery action", http.StatusNotFound)
		return
	}
	runID := parts[1]
	var (
		response *RecoverRoundResponse
		err      error
	)
	switch {
	case len(parts) == 5 && parts[2] == "seats" && parts[4] == "retry":
		response, err = h.svc.RetryRoundSeat(roomID, runID, Role(parts[3]))
	case len(parts) == 3 && parts[2] == "skip":
		response, err = h.svc.SkipFailedSeatsAndSummarize(roomID, runID)
	case len(parts) == 4 && parts[2] == "summary" && parts[3] == "retry":
		response, err = h.svc.RetryRoundSummary(roomID, runID)
	default:
		http.Error(w, "unknown recovery action", http.StatusNotFound)
		return
	}
	if err != nil {
		writeRecoveryErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, response)
}

func writeRecoveryErr(w http.ResponseWriter, err error) {
	if errors.Is(err, meta.ErrNotFound) {
		http.Error(w, "run or room not found", http.StatusNotFound)
		return
	}
	message := err.Error()
	if strings.Contains(message, "unavailable") ||
		strings.Contains(message, "does not belong") ||
		strings.Contains(message, "invalid panelist") ||
		strings.Contains(message, "no failed seats") {
		http.Error(w, message, http.StatusConflict)
		return
	}
	http.Error(w, message, http.StatusInternalServerError)
}

func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if q := strings.TrimSpace(r.URL.Query().Get("limit")); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	rooms, err := h.svc.ListRooms(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"rooms": rooms})
}

func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request) {
	var req CreateRoomRequest
	if r.Body != nil {
		defer r.Body.Close()
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	room, err := h.svc.CreateRoom(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, room)
}

func (h *Handler) getRoom(w http.ResponseWriter, r *http.Request, id string) {
	room, err := h.svc.GetRoom(id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, room)
}

func (h *Handler) listSeats(w http.ResponseWriter, r *http.Request, id string) {
	seats, err := h.svc.ListSeats(id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"seats": seats})
}

func (h *Handler) listTurns(w http.ResponseWriter, r *http.Request, id string) {
	turns, err := h.svc.ListTurns(id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"turns": turns})
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request, id string) {
	afterText := strings.TrimSpace(r.URL.Query().Get("after"))
	if afterText == "" {
		afterText = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	var after int64
	if afterText != "" {
		value, err := strconv.ParseInt(afterText, 10, 64)
		if err != nil || value < 0 {
			http.Error(w, "after must be a non-negative event sequence", http.StatusBadRequest)
			return
		}
		after = value
	}
	limit := 200
	if text := strings.TrimSpace(r.URL.Query().Get("limit")); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value <= 0 {
			http.Error(w, "limit must be positive", http.StatusBadRequest)
			return
		}
		limit = value
	}
	events, err := h.svc.ListRoundEvents(id, after, limit)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	lastSeq := after
	if len(events) > 0 {
		lastSeq = events[len(events)-1].Seq
	}
	w.Header().Set("X-Last-Event-Seq", strconv.FormatInt(lastSeq, 10))
	writeJSON(w, map[string]any{
		"events":   events,
		"last_seq": lastSeq,
	})
}

func (h *Handler) chat(w http.ResponseWriter, r *http.Request, id string) {
	var req ChatRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	resp, err := h.svc.ChatWithReferee(id, req)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		msg := err.Error()
		// Validation / wrong-state → 400; agent/bridge failures → 502.
		if strings.Contains(msg, "required") || strings.Contains(msg, "only allowed") {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if strings.Contains(msg, "bridge unavailable") || strings.Contains(msg, "referee prompt") {
			http.Error(w, msg, http.StatusBadGateway)
			return
		}
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (h *Handler) confirmBrief(w http.ResponseWriter, r *http.Request, id string) {
	var req ConfirmBriefRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	room, err := h.svc.ConfirmBrief(id, req)
	if err != nil {
		writeBriefMutationErr(w, err)
		return
	}
	writeJSON(w, room)
}

func (h *Handler) saveBriefDraft(w http.ResponseWriter, r *http.Request, id string) {
	var req SaveBriefDraftRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	room, err := h.svc.SaveBriefDraft(id, req)
	if err != nil {
		writeBriefMutationErr(w, err)
		return
	}
	writeJSON(w, room)
}

func (h *Handler) proposeBrief(w http.ResponseWriter, r *http.Request, id string) {
	var req ProposeBriefRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	room, err := h.svc.ProposeBrief(id, req)
	if err != nil {
		writeBriefMutationErr(w, err)
		return
	}
	writeJSON(w, room)
}

func (h *Handler) confirmBriefVersion(w http.ResponseWriter, r *http.Request, id string) {
	var req ConfirmBriefVersionRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	room, err := h.svc.ConfirmBriefVersion(id, req)
	if err != nil {
		writeBriefMutationErr(w, err)
		return
	}
	writeJSON(w, room)
}

func writeBriefMutationErr(w http.ResponseWriter, err error) {
	if errors.Is(err, meta.ErrNotFound) {
		http.Error(w, "room or brief version not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, ErrBriefVersionConflict) {
		payload := map[string]any{
			"code":    "brief_version_conflict",
			"message": err.Error(),
		}
		var conflict *BriefVersionConflictError
		if errors.As(err, &conflict) {
			payload["expected_version"] = conflict.Expected
			payload["current_version"] = conflict.Current
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, payload)
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "required") || strings.Contains(msg, "must be") ||
		strings.Contains(msg, "only in") || strings.Contains(msg, "cannot") ||
		strings.Contains(msg, "running") || strings.Contains(msg, "status=") ||
		strings.Contains(msg, "illegal") {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

func (h *Handler) runR2(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	start, err := h.svc.StartR2(id, req.IdempotencyKey)
	if err != nil {
		writeRoundStartErr(w, err)
		return
	}
	if r.URL.Query().Get("wait") == "1" {
		run, err := h.svc.WaitRoundRun(r.Context(), start.RunID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout)
			return
		}
		if run.Status == RunFailed || run.Status == RunCanceled {
			http.Error(w, run.Error, http.StatusBadGateway)
			return
		}
		response, err := h.svc.buildRunR2Response(id)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		writeJSON(w, response)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, start)
}

func (h *Handler) runR3(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	start, err := h.svc.StartR3(id, req.IdempotencyKey)
	if err != nil {
		writeRoundStartErr(w, err)
		return
	}
	if r.URL.Query().Get("wait") == "1" {
		run, err := h.svc.WaitRoundRun(r.Context(), start.RunID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout)
			return
		}
		if run.Status == RunFailed || run.Status == RunCanceled {
			http.Error(w, run.Error, http.StatusBadGateway)
			return
		}
		response, err := h.svc.buildRunR3Response(id)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		writeJSON(w, response)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, start)
}

func writeRoundStartErr(w http.ResponseWriter, err error) {
	if errors.Is(err, meta.ErrNotFound) {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "only allowed") || strings.Contains(msg, "required") ||
		strings.Contains(msg, "brief") || strings.Contains(msg, "round must") {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

func writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, meta.ErrNotFound) {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
