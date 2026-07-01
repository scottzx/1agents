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
	ccs    *meta.FeishuChatStore // tracked chats + global sync config (v17)
	cs     *meta.ContactStore    // 二度联系人: roster ingestion after each sync (v18)
	syncer *feishu.Syncer
}

// NewHandler builds a Handler from explicit stores (used by tests). cs may be
// nil in tests that don't exercise the channel-config endpoints. The contact
// store is left nil here (member ingestion is wired only in NewHandlerDefault).
func NewHandler(fs *feishu.Store, ds *meta.DigestStore, ts *meta.TaskStore, cs *meta.FeishuChatStore, syncer *feishu.Syncer) *Handler {
	return &Handler{fs: fs, ds: ds, ts: ts, ccs: cs, syncer: syncer}
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
	h := NewHandler(fs, meta.NewDigestStore(db), meta.NewTaskStore(db),
		meta.NewFeishuChatStore(db),
		feishu.NewSyncer(fs, feishu.NewClient("", "self")))
	h.cs = meta.NewContactStore(db) // wire 二度联系人 roster ingestion
	return h, nil
}

// ensureRoster fetches a tracked chat's full chat.members roster exactly ONCE
// (the first sync), ingests it as degree-2 contacts + the name cache, then sets
// members_fetched so later syncs make ZERO chat.members calls. Best-effort:
// errors are logged, never propagated. No-op when stores are unwired (tests) or
// when the roster was already fetched.
func (h *Handler) ensureRoster(ctx context.Context, chatID string) {
	if h.cs == nil || h.ccs == nil {
		return
	}
	tc, ok, err := h.ccs.GetTrackedChat(chatID)
	if err != nil {
		log.Printf("[digest] roster %s: lookup tracked: %v", chatID, err)
		return
	}
	if ok && tc.MembersFetched {
		return // roster already cached — never fetch chat.members again
	}
	members, total, err := h.client().FetchMembersDetailed(ctx, chatID)
	if err != nil {
		log.Printf("[digest] roster %s: fetch members: %v", chatID, err)
		return
	}
	gm := make([]meta.GroupMember, 0, len(members))
	for _, m := range members {
		gm = append(gm, meta.GroupMember{OpenID: m.OpenID, Name: m.Name, TenantKey: m.TenantKey})
	}
	if _, err := h.cs.IngestGroupMembers(chatID, gm); err != nil {
		log.Printf("[digest] roster %s: ingest members: %v", chatID, err)
		return // don't flag as fetched if ingestion failed — retry next sync
	}
	// Record the chat's true size (member_total). For large groups the API caps
	// the enumerable roster, so total > len(members); store it so the UI shows
	// real group size alongside how many were ingested.
	if err := h.ccs.SetTrackedMemberTotal(chatID, total); err != nil {
		log.Printf("[digest] roster %s: set member_total: %v", chatID, err)
	}
	if err := h.ccs.SetMembersFetched(chatID); err != nil {
		log.Printf("[digest] roster %s: set members_fetched: %v", chatID, err)
	}
}

// syncOne runs one full sync for a chat through the optimized pipeline:
//  1. ensureRoster — fetch chat.members ONCE on the first sync (flag-gated);
//  2. build the name map from the cached roster (no chat.members API call);
//  3. SyncChat with that name map (sender-name enrichment, no roster call);
//  4. incrementally ingest the batch's distinct senders (open_id + tenant_key)
//     as degree-2 contacts — capturing active speakers beyond the roster cap.
//
// The caller owns the last_synced_at bump (UpdateTrackedSynced) since the two
// HTTP paths and the periodic loop word it differently.
func (h *Handler) syncOne(ctx context.Context, chatID string) (feishu.SyncResult, error) {
	h.ensureRoster(ctx, chatID)

	var names map[string]string
	if h.cs != nil {
		var err error
		if names, err = h.cs.RosterNameMap(chatID); err != nil {
			log.Printf("[digest] roster %s: name map: %v", chatID, err)
		}
	}

	res, err := h.syncer.SyncChat(ctx, chatID, names)
	if err != nil {
		return res, err
	}

	// Incrementally capture this batch's active speakers (beyond the roster cap).
	if h.cs != nil && len(res.Senders) > 0 {
		senders := make([]meta.SenderRef, 0, len(res.Senders))
		for _, sr := range res.Senders {
			senders = append(senders, meta.SenderRef{OpenID: sr.OpenID, TenantKey: sr.TenantKey})
		}
		if _, err := h.cs.IngestMessageSenders(chatID, senders); err != nil {
			log.Printf("[digest] roster %s: ingest senders: %v", chatID, err)
		}
	}
	return res, nil
}

// client reaches the lark-cli client through the syncer (chat listing / doctor).
func (h *Handler) client() *feishu.Client { return h.syncer.Client() }

// Seed inserts the built-in presets if absent.
func (h *Handler) Seed() error { return Seed(h.ds) }

// periodicBaseTick is how often the periodic loop wakes to re-check the live
// config. It is intentionally small (vs the multi-hour sync interval) so a
// changed enable/interval takes effect without a server restart. The interval
// argument passed to StartPeriodicSync is ignored in favor of the DB config.
const periodicBaseTick = 15 * time.Minute

// StartPeriodicSync ticks at periodicBaseTick and on each tick re-reads the live
// sync config: when enabled, it syncs every auto-sync tracked chat whose
// last_synced_at is older than interval_minutes. This makes enable/interval
// changes (and track/untrack) take effect without a restart. The interval arg
// is accepted for call-site compatibility but ignored. Best-effort: errors are
// logged, not fatal — Feishu history is not time-critical.
func (h *Handler) StartPeriodicSync(ctx context.Context, _ time.Duration) {
	if h.ccs == nil {
		return
	}
	go func() {
		t := time.NewTicker(periodicBaseTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				h.syncDueTracked(ctx)
			}
		}
	}()
}

// syncDueTracked syncs the auto-sync tracked chats whose cadence is due.
func (h *Handler) syncDueTracked(ctx context.Context) {
	cfg, err := h.ccs.GetSyncConfig()
	if err != nil {
		log.Printf("[digest] periodic sync: read config: %v", err)
		return
	}
	if !cfg.Enabled {
		return
	}
	chats, err := h.ccs.ListTrackedChats(true)
	if err != nil {
		log.Printf("[digest] periodic sync: list tracked: %v", err)
		return
	}
	dueBefore := time.Now().Add(-time.Duration(cfg.IntervalMinutes) * time.Minute).UnixMilli()
	for _, c := range chats {
		if c.LastSyncedAt > dueBefore {
			continue // synced recently enough
		}
		res, err := h.syncOne(ctx, c.ChatID)
		if err != nil {
			log.Printf("[digest] periodic sync %s: %v", c.ChatID, err)
			continue
		}
		if err := h.ccs.UpdateTrackedSynced(c.ChatID, "", time.Now().UnixMilli()); err != nil {
			log.Printf("[digest] periodic sync %s: update synced: %v", c.ChatID, err)
		}
		if res.Inserted > 0 {
			log.Printf("[digest] periodic sync %s: +%d messages", c.ChatID, res.Inserted)
		}
	}
}

// SyncTracked syncs every auto-sync tracked chat now, ignoring the per-chat due
// check — the work-order scheduler's interval is the cadence, so this is the
// entry point the ingest sync task calls in place of the retired periodic
// ticker (StartPeriodicSync). It reuses the proven syncOne path (roster fetch +
// message upsert into unified_messages + 二度联系人 sender ingestion) and bumps
// last_synced_at, so the existing message/digest UI stays fully intact. Returns
// how many chats were synced and total messages inserted. Best-effort per chat:
// a single chat's failure is logged and skipped.
func (h *Handler) SyncTracked(ctx context.Context) (chats int, inserted int, err error) {
	if h.ccs == nil {
		return 0, 0, nil
	}
	tracked, err := h.ccs.ListTrackedChats(true)
	if err != nil {
		return 0, 0, err
	}
	now := time.Now().UnixMilli()
	for _, c := range tracked {
		res, e := h.syncOne(ctx, c.ChatID)
		if e != nil {
			log.Printf("[digest] work-order sync %s: %v", c.ChatID, e)
			continue
		}
		if e := h.ccs.UpdateTrackedSynced(c.ChatID, "", now); e != nil {
			log.Printf("[digest] work-order sync %s: update synced: %v", c.ChatID, e)
		}
		chats++
		inserted += res.Inserted
	}
	return chats, inserted, nil
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
	res, err := h.syncOne(r.Context(), body.ChatID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// Keep last_synced_at fresh if this chat is tracked (no-op otherwise).
	if h.ccs != nil {
		_ = h.ccs.UpdateTrackedSynced(body.ChatID, "", time.Now().UnixMilli())
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

// ── 飞书渠道配置 (Phase 2) ──────────────────────────────────────────────────

// HandleAvailableChats: GET → the Feishu groups the user is in (via lark-cli).
func (h *Handler) HandleAvailableChats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	chats, err := h.client().ListChats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, chats)
}

// HandleTrackedChats: GET list tracked; POST {chatId, chatName, avatar, external}
// to start tracking.
func (h *Handler) HandleTrackedChats(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.ccs.ListTrackedChats(false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Attach the degree-2 roster size per chat (additive: TrackedChat fields
		// inline + memberCount). Zero when the roster hasn't been ingested yet.
		type trackedWithMembers struct {
			meta.TrackedChat
			MemberCount int `json:"memberCount"`
		}
		out := make([]trackedWithMembers, 0, len(list))
		for _, c := range list {
			n := 0
			if h.cs != nil {
				n, _ = h.cs.MemberCountForSession(c.ChatID)
			}
			out = append(out, trackedWithMembers{TrackedChat: c, MemberCount: n})
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var body struct {
			ChatID   string `json:"chatId"`
			ChatName string `json:"chatName"`
			Avatar   string `json:"avatar"`
			External bool   `json:"external"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ChatID == "" {
			badRequest(w, "chatId required")
			return
		}
		if err := h.ccs.UpsertTrackedChat(meta.TrackedChat{
			ChatID:   body.ChatID,
			ChatName: body.ChatName,
			Avatar:   body.Avatar,
			External: body.External,
			AutoSync: true,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		t, _, _ := h.ccs.GetTrackedChat(body.ChatID)
		writeJSON(w, http.StatusCreated, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTrackedChatItem: DELETE untrack; PATCH {autoSync} toggle auto-sync.
// Path: /api/digest/chats/tracked/{chatId}.
func (h *Handler) HandleTrackedChatItem(w http.ResponseWriter, r *http.Request) {
	chatID := strings.TrimPrefix(r.URL.Path, "/api/digest/chats/tracked/")
	if chatID == "" || strings.Contains(chatID, "/") {
		badRequest(w, "chatId required")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := h.ccs.DeleteTrackedChat(chatID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		var body struct {
			AutoSync bool `json:"autoSync"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		if err := h.ccs.SetTrackedAutoSync(chatID, body.AutoSync); err != nil {
			if err == meta.ErrNotFound {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		t, _, _ := h.ccs.GetTrackedChat(chatID)
		writeJSON(w, http.StatusOK, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSyncAll: POST → sync every tracked chat now; returns per-chat results.
func (h *Handler) HandleSyncAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	chats, err := h.ccs.ListTrackedChats(false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type chatResult struct {
		ChatID   string `json:"chatId"`
		Inserted int    `json:"inserted"`
		Fetched  int    `json:"fetched"`
		Error    string `json:"error,omitempty"`
	}
	results := make([]chatResult, 0, len(chats))
	for _, c := range chats {
		res, err := h.syncOne(r.Context(), c.ChatID)
		if err != nil {
			results = append(results, chatResult{ChatID: c.ChatID, Error: err.Error()})
			continue
		}
		_ = h.ccs.UpdateTrackedSynced(c.ChatID, "", time.Now().UnixMilli())
		results = append(results, chatResult{ChatID: c.ChatID, Inserted: res.Inserted, Fetched: res.Fetched})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// HandleStatus: GET → lark-cli connectivity (doctor checks) + the sync config.
// On a doctor hard failure, connected is false and error carries the message.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := h.ccs.GetSyncConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	checks, derr := h.client().Doctor(r.Context())
	if derr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"error":     derr.Error(),
			"checks":    []any{},
			"config":    cfg,
		})
		return
	}
	connected := true
	for _, c := range checks {
		if c.Status == "fail" {
			connected = false
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connected": connected,
		"checks":    checks,
		"config":    cfg,
	})
}

// HandleSyncConfig: GET current config; PUT {enabled, intervalMinutes} to set.
func (h *Handler) HandleSyncConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := h.ccs.GetSyncConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		var body struct {
			Enabled         bool `json:"enabled"`
			IntervalMinutes int  `json:"intervalMinutes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		if err := h.ccs.SetSyncConfig(body.Enabled, body.IntervalMinutes); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg, _ := h.ccs.GetSyncConfig()
		writeJSON(w, http.StatusOK, cfg)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
