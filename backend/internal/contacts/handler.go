// Package contacts serves the 联系人聚合 HTTP API: a user-curated address book
// (meta.db v16) that auto-discovers channel identities from synced Feishu
// messages (sync.db). Feishu can't return phones for external group members, so
// the user creates contacts (phone = unique merge key) and binds discovered
// channel identities to them. v1 is Feishu-only; the model carries a platform
// discriminator for future WeChat/email.
package contacts

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Handler serves the contacts HTTP API over the address book (meta.db) and the
// synced message store (sync.db). It wires its own stores from the default
// databases so server.go stays thin.
type Handler struct {
	fs  *feishu.Store
	cs  *meta.ContactStore
	ccs *meta.FeishuChatStore     // tracked-chat names overlay onto session summaries
	cps *meta.CompanyStore        // tenant_key→org-name map for the contacts grid/modal
	cms *meta.ChannelModuleStore  // per-sub-module consent + crawl rules (privacy gate)
}

// NewHandler builds a Handler from explicit stores (used by tests). ccs may be
// nil; session summaries then keep the chat_id fallback for names. cps may be
// nil; the companies endpoint then returns an empty map.
func NewHandler(fs *feishu.Store, cs *meta.ContactStore, ccs *meta.FeishuChatStore, cps *meta.CompanyStore, cms *meta.ChannelModuleStore) *Handler {
	if cps != nil {
		// Idempotent: ensure 飞书官方 is seeded so the frontend's org labels resolve
		// without the old hardcoded constant. Log + ignore so a seed failure never
		// blocks the contacts API.
		if err := cps.SeedFeishuOfficial(); err != nil {
			log.Printf("[contacts] seed 飞书官方 failed: %v", err)
		}
	}
	return &Handler{fs: fs, cs: cs, ccs: ccs, cps: cps, cms: cms}
}

// NewHandlerDefault wires the handler from the default sync.db + meta.db (the
// same cached handles the digest module uses). Returns an error if either store
// can't be opened.
func NewHandlerDefault() (*Handler, error) {
	fs, err := feishu.OpenDefault()
	if err != nil {
		return nil, err
	}
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	return NewHandler(fs, meta.NewContactStore(db), meta.NewFeishuChatStore(db), meta.NewCompanyStore(db), meta.NewChannelModuleStore(db)), nil
}

// ── HTTP helpers ──

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func badRequest(w http.ResponseWriter, msg string) { http.Error(w, msg, http.StatusBadRequest) }

type contactBody struct {
	Phone   string   `json:"phone"`
	Name    string   `json:"name"`
	Company string   `json:"company"`
	Title   string   `json:"title"`
	Note    string   `json:"note"`
	Tags    []string `json:"tags"`
}

// HandleContacts: GET list (each with bound channels); POST create.
func (h *Handler) HandleContacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Optional ?degree=1|2 filter; absent/0 returns all degrees.
		degree := 0
		if v := r.URL.Query().Get("degree"); v != "" {
			degree, _ = strconv.Atoi(v)
		}
		list, err := h.cs.ContactsWithChannelsByDegree(degree)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var body contactBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		c, err := h.cs.CreateContact(body.Phone, body.Name, body.Company, body.Title, body.Note, body.Tags)
		if err == meta.ErrDuplicatePhone {
			http.Error(w, "phone already used by another contact", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleContactItem: PATCH update; DELETE remove. Path: /api/contacts/{id}.
func (h *Handler) HandleContactItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/contacts/")
	// GET /api/contacts/{id}/groups → the tracked-group sessionIds this contact
	// belongs to, derived from the roster. A person in N groups has ONE open_id
	// channel (whose single session_id is just last-seen) but N roster rows, so
	// membership must come from the roster, not the channel.
	if cid, ok := strings.CutSuffix(rest, "/groups"); ok {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		chans, err := h.cs.ListChannelsForContact(cid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		seen := map[string]bool{}
		sessions := []string{}
		for _, ch := range chans {
			if ch.Platform != feishu.Channel {
				continue
			}
			groups, err := h.cs.GroupsForChannel(ch.ChannelID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, sid := range groups {
				if !seen[sid] {
					seen[sid] = true
					sessions = append(sessions, sid)
				}
			}
		}
		writeJSON(w, http.StatusOK, sessions)
		return
	}
	id := rest
	if id == "" || strings.Contains(id, "/") {
		badRequest(w, "contact id required")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var body contactBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		c, err := h.cs.UpdateContact(id, body.Phone, body.Name, body.Company, body.Title, body.Note, body.Tags)
		if err == meta.ErrDuplicatePhone {
			http.Error(w, "phone already used by another contact", http.StatusConflict)
			return
		}
		if err == meta.ErrNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, c)
	case http.MethodDelete:
		if err := h.cs.DeleteContact(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleChannels: GET ?contactId=&unlinked=1 → discovered channel identities.
func (h *Handler) HandleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	contactID := r.URL.Query().Get("contactId")
	unlinked := r.URL.Query().Get("unlinked") == "1"
	list, err := h.cs.ListChannels(contactID, unlinked)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// HandleDiscover: POST → scan synced Feishu messages for distinct senders and
// upsert each as a channel identity. Returns {discovered, updated}: discovered
// is the count of distinct senders found; updated is the count of successful
// upserts (new or refreshed) — they coincide on a clean run, but updated
// reflects rows actually written.
func (h *Handler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	senders, err := h.fs.DistinctSenders(feishu.Channel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	updated := 0
	for _, s := range senders {
		if err := h.cs.UpsertChannel(feishu.Channel, s.SenderID, s.SenderName, s.SessionID, s.LastSeen); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		updated++
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"discovered": len(senders),
		"updated":    updated,
	})
}

// HandleChannelAction: POST /api/contacts/channels/{id}/link {contactId} or
// /api/contacts/channels/{id}/unlink.
func (h *Handler) HandleChannelAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/contacts/channels/")
	id, action, ok := strings.Cut(rest, "/")
	if !ok || id == "" {
		badRequest(w, "channel id and action required")
		return
	}
	switch action {
	case "link":
		var body struct {
			ContactID string `json:"contactId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ContactID == "" {
			badRequest(w, "contactId required")
			return
		}
		if err := h.cs.LinkChannel(id, body.ContactID); err != nil {
			if err == meta.ErrNotFound {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "unlink":
		if err := h.cs.UnlinkChannel(id); err != nil {
			if err == meta.ErrNotFound {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		badRequest(w, "unknown action")
	}
}

// HandleMessages: GET ?contactId= → a contact's cross-group messages (gather its
// channel_ids → MessagesBySenders); or ?sessionId= → one session's messages.
func (h *Handler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil {
			limit = l
		}
	}
	sessionID := r.URL.Query().Get("sessionId")
	contactID := r.URL.Query().Get("contactId")
	// contactId present → this contact's messages; sessionId (optional) narrows to
	// one group. contactId + sessionId = "this person in that group"; contactId
	// alone = merged cross-group. sessionId alone (no contactId) = the whole
	// group's messages (used by the 消息 tab's session view).
	if contactID != "" {
		chans, err := h.cs.ListChannelsForContact(contactID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		senderIDs := make([]string, 0, len(chans))
		for _, c := range chans {
			if c.Platform == feishu.Channel {
				senderIDs = append(senderIDs, c.ChannelID)
			}
		}
		msgs, err := h.fs.MessagesBySenders(feishu.Channel, senderIDs, sessionID, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, msgs)
		return
	}
	if sessionID != "" {
		msgs, err := h.fs.ListMessages(feishu.Channel, sessionID, 0, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, msgs)
		return
	}
	badRequest(w, "contactId or sessionId required")
}

// HandleGroupMembers: GET /api/contacts/groups/{sessionId}/members → the recorded
// degree-2 roster (open_id + nickname + tenant_key) of a tracked group.
func (h *Handler) HandleGroupMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/contacts/groups/")
	sessionID, tail, ok := strings.Cut(rest, "/")
	if !ok || sessionID == "" || tail != "members" {
		badRequest(w, "path must be /api/contacts/groups/{sessionId}/members")
		return
	}
	members, err := h.cs.GroupMembersForSession(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// HandleSessions: GET → session summaries (group list for the 消息 tab).
func (h *Handler) HandleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sums, err := h.fs.SessionSummaries(feishu.Channel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Overlay tracked-chat names so the 消息 tab shows human group names instead
	// of raw chat_ids (the only display-name source; sync.db has none). Missing
	// names keep the chat_id fallback already set by SessionSummaries.
	if h.ccs != nil {
		if names, nerr := h.ccs.TrackedNamesBySession(); nerr == nil {
			for i := range sums {
				if n := names[sums[i].SessionID]; n != "" {
					sums[i].SessionName = n
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, sums)
}

// HandleCompanies: GET → the tenant_key→company-name map (company_tenants ×
// companies). The frontend builds a tenantKey→shortName lookup to label each
// contact's channel org, replacing the old hardcoded 飞书官方 constant. Returns an
// empty list when no company store is wired.
func (h *Handler) HandleCompanies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cps == nil {
		writeJSON(w, http.StatusOK, []meta.CompanyTenant{})
		return
	}
	list, err := h.cps.TenantCompanyMap()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
