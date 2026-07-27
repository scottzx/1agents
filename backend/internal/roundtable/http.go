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
//	POST /api/roundtable/rooms/{id}/brief   — confirm Brief → waiting_r2
//	POST /api/roundtable/rooms/{id}/r2      — R2 parallel speeches + Summary₂ → waiting_r3
//	POST /api/roundtable/rooms/{id}/r3      — R3 resume + public context + Summary₃ → done
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
	if r.Body != nil {
		defer r.Body.Close()
		// Drain optional empty body.
		_, _ = io.Copy(io.Discard, r.Body)
	}
	resp, err := h.svc.RunR2(id)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		msg := err.Error()
		if strings.Contains(msg, "only allowed") || strings.Contains(msg, "required") ||
			strings.Contains(msg, "brief") {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if strings.Contains(msg, "bridge unavailable") || strings.Contains(msg, "r2 summary") ||
			strings.Contains(msg, "agent error") {
			http.Error(w, msg, http.StatusBadGateway)
			return
		}
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (h *Handler) runR3(w http.ResponseWriter, r *http.Request, id string) {
	if r.Body != nil {
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
	}
	resp, err := h.svc.RunR3(id)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		msg := err.Error()
		if strings.Contains(msg, "only allowed") || strings.Contains(msg, "required") ||
			strings.Contains(msg, "brief") || strings.Contains(msg, "summary_r2") {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if strings.Contains(msg, "bridge unavailable") || strings.Contains(msg, "r3 summary") ||
			strings.Contains(msg, "agent error") {
			http.Error(w, msg, http.StatusBadGateway)
			return
		}
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
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
