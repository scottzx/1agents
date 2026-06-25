package digest

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Handler serves the chat-digest HTTP API and owns the periodic Feishu sync.
// It wires its own stores from the default databases so server.go stays thin.
type Handler struct {
	fs     *feishu.Store
	ds     *meta.DigestStore
	ts     *meta.TaskStore
	syncer *feishu.Syncer
}

// NewHandler builds a Handler from explicit stores (used by tests).
func NewHandler(fs *feishu.Store, ds *meta.DigestStore, ts *meta.TaskStore, syncer *feishu.Syncer) *Handler {
	return &Handler{fs: fs, ds: ds, ts: ts, syncer: syncer}
}

// NewHandlerDefault wires the handler from the default sync.db + meta.db and a
// lark-cli-backed syncer. Returns an error if either store can't be opened.
func NewHandlerDefault() (*Handler, error) {
	fs, err := feishu.OpenDefault()
	if err != nil {
		return nil, err
	}
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	return NewHandler(fs, meta.NewDigestStore(db), meta.NewTaskStore(db),
		feishu.NewSyncer(fs, feishu.NewClient("", "self"))), nil
}

// Seed inserts the built-in presets if absent.
func (h *Handler) Seed() error { return Seed(h.ds) }

// StartPeriodicSync re-syncs every already-known chat on the given interval
// (best-effort; errors are logged, not fatal). Feishu history is not
// time-critical, so a multi-hour cadence is fine.
func (h *Handler) StartPeriodicSync(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				h.syncAllKnown(ctx)
			}
		}
	}()
}

func (h *Handler) syncAllKnown(ctx context.Context) {
	sessions, err := h.fs.ListSessions(feishu.Channel)
	if err != nil {
		log.Printf("[digest] periodic sync: list sessions: %v", err)
		return
	}
	for _, sid := range sessions {
		if res, err := h.syncer.SyncChat(ctx, sid); err != nil {
			log.Printf("[digest] periodic sync %s: %v", sid, err)
		} else if res.Inserted > 0 {
			log.Printf("[digest] periodic sync %s: +%d messages", sid, res.Inserted)
		}
	}
}

// ── HTTP helpers ──

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func badRequest(w http.ResponseWriter, msg string) { http.Error(w, msg, http.StatusBadRequest) }

// HandleTemplates: GET list all; POST create {name, scope, bodyMd, isDefault}.
func (h *Handler) HandleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tpls, err := h.ds.ListTemplates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, tpls)
	case http.MethodPost:
		var body struct {
			Name      string `json:"name"`
			Scope     string `json:"scope"`
			BodyMD    string `json:"bodyMd"`
			IsDefault bool   `json:"isDefault"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			badRequest(w, "name required")
			return
		}
		t, err := h.ds.CreateTemplate(body.Name, body.Scope, body.BodyMD, body.IsDefault)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTemplateItem: PATCH {bodyMd} edit body; DELETE remove. Path:
// /api/digest/templates/{id}.
func (h *Handler) HandleTemplateItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/digest/templates/")
	if id == "" || strings.Contains(id, "/") {
		badRequest(w, "template id required")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var body struct {
			BodyMD string `json:"bodyMd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		if err := h.ds.UpdateTemplateBody(id, body.BodyMD); err != nil {
			if err == meta.ErrNotFound {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		t, _, _ := h.ds.GetTemplate(id)
		writeJSON(w, http.StatusOK, t)
	case http.MethodDelete:
		if err := h.ds.DeleteTemplate(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleBindings: GET ?session= list templates a chat resolves to; POST
// {sessionId, templateId} bind; DELETE ?session=&template= unbind.
func (h *Handler) HandleBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		session := r.URL.Query().Get("session")
		if session == "" {
			badRequest(w, "session required")
			return
		}
		tpls, err := h.ds.TemplatesForSession(session)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, tpls)
	case http.MethodPost:
		var body struct {
			SessionID  string `json:"sessionId"`
			TemplateID string `json:"templateId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID == "" || body.TemplateID == "" {
			badRequest(w, "sessionId and templateId required")
			return
		}
		if err := h.ds.BindTemplate(body.SessionID, body.TemplateID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		session := r.URL.Query().Get("session")
		template := r.URL.Query().Get("template")
		if session == "" || template == "" {
			badRequest(w, "session and template required")
			return
		}
		if err := h.ds.UnbindTemplate(session, template); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSync: POST {chatId} → run one incremental sync now.
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ChatID string `json:"chatId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ChatID == "" {
		badRequest(w, "chatId required")
		return
	}
	res, err := h.syncer.SyncChat(r.Context(), body.ChatID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// HandleAnalyze: POST {chatId, chatName, workspace, sinceMs?} → assemble the
// batch + resolved templates into a scheduler-eligible analysis task. The
// existing scheduler/runner then executes it and the agent's extraction lands
// on the task timeline.
func (h *Handler) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ChatID    string `json:"chatId"`
		ChatName  string `json:"chatName"`
		Workspace string `json:"workspace"`
		SinceMs   int64  `json:"sinceMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ChatID == "" || body.Workspace == "" {
		badRequest(w, "chatId and workspace required")
		return
	}
	if body.ChatName == "" {
		body.ChatName = body.ChatID
	}
	prompt, n, err := PrepareBatch(h.fs, h.ds, feishu.Channel, body.ChatID, body.ChatName, body.SinceMs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n == 0 {
		badRequest(w, "no messages to analyze (sync first?)")
		return
	}
	task, err := CreateAnalysisTask(h.ts, body.Workspace, body.ChatName, prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"taskId":       task.ID,
		"number":       task.Number,
		"messageCount": n,
	})
}

// HandleMessages: GET ?session=&sinceMs=&limit= → stored messages (inspection).
func (h *Handler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := r.URL.Query().Get("session")
	if session == "" {
		badRequest(w, "session required")
		return
	}
	var since int64
	if v := r.URL.Query().Get("sinceMs"); v != "" {
		since, _ = strconv.ParseInt(v, 10, 64)
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil {
			limit = l
		}
	}
	msgs, err := h.fs.ListMessages(feishu.Channel, session, since, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}
