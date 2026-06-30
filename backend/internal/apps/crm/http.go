package crm

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Handler serves the CRM L1 page REST API. Construct it with NewHandler and wire
// the routes from the server (orchestrator central wiring); the package itself
// imports no router.
//
// Routes (to be mounted by the orchestrator):
//
//	GET  /api/crm/contacts            → list contacts
//	POST /api/crm/contacts            → create a contact (body = Contact)
//	POST /api/crm/contacts/parse-card → parse business-card text → contact (body {text})
//	POST /api/crm/ingest              → pull inbox items → contacts (#340)
//	GET  /api/crm/leads               → list leads (each with inline tasks)
//	POST /api/crm/leads               → create a lead (body {contactId, owner?, notes?})
//	POST /api/crm/leads/{id}/score    → dispatch agent mining/scoring (#341) {workspacePath, context?}
//	POST /api/crm/leads/{id}/enrich   → dispatch function enrichment (#342) {workspacePath}
//	POST /api/crm/leads/{id}/follow   → human follow decision → stage advance (#343) {workspacePath}
//	POST /api/crm/leads/{id}/drop     → human drop decision (#343) {workspacePath}
type Handler struct {
	store *Store
	inbox *meta.InboxStore
}

// NewHandler builds a CRM HTTP handler over the shared meta.db.
func NewHandler(db *meta.DB) *Handler {
	return &Handler{
		store: NewStore(db.SQL()),
		inbox: meta.NewInboxStore(db),
	}
}

// HandleContacts serves GET/POST /api/crm/contacts.
func (h *Handler) HandleContacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.store.ListContacts()
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"contacts": list})
	case http.MethodPost:
		var c Contact
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(c.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if c.Source == "" {
			c.Source = "manual"
		}
		saved, err := h.store.UpsertContact(c)
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleParseCard serves POST /api/crm/contacts/parse-card {text} → Contact.
// It parses (and saves) a contact from business-card text (#340).
func (h *Handler) HandleParseCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Text string `json:"text"`
		Save bool   `json:"save"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	c, ok := ParseBusinessCard(body.Text)
	if !ok {
		http.Error(w, "no contact fields found in card text", http.StatusUnprocessableEntity)
		return
	}
	if body.Save {
		saved, err := h.store.UpsertContact(c)
		if err != nil {
			httpErr(w, err)
			return
		}
		c = saved
	}
	writeJSON(w, c)
}

// HandleIngest serves POST /api/crm/ingest → pull inbox items into contacts (#340).
func (h *Handler) HandleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := h.store.IngestFromInbox(h.inbox)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ingested": n})
}

// leadView is a lead plus its inline task execution state (#343).
type leadView struct {
	Lead
	Tasks []taskBrief `json:"tasks"`
}

type taskBrief struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Executor string `json:"executor"`
}

// HandleLeads serves GET/POST /api/crm/leads.
func (h *Handler) HandleLeads(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		leads, err := h.store.ListLeads()
		if err != nil {
			httpErr(w, err)
			return
		}
		out := make([]leadView, 0, len(leads))
		for _, l := range leads {
			out = append(out, leadView{Lead: l, Tasks: tasksForLead(l.ID)})
		}
		writeJSON(w, map[string]any{"leads": out})
	case http.MethodPost:
		var body struct {
			ContactID string `json:"contactId"`
			Owner     string `json:"owner"`
			Notes     string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.ContactID) == "" {
			http.Error(w, "contactId is required", http.StatusBadRequest)
			return
		}
		saved, err := h.store.CreateLead(Lead{ContactID: body.ContactID, Owner: body.Owner, Notes: body.Notes})
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleLeadAction serves POST /api/crm/leads/{id}/{action}.
// action ∈ { score, enrich, follow, drop }.
func (h *Handler) HandleLeadAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, action := splitLeadAction(r.URL.Path)
	if id == "" || action == "" {
		http.Error(w, "missing lead id or action", http.StatusBadRequest)
		return
	}
	if _, ok, _ := h.store.GetLead(id); !ok {
		http.Error(w, "lead not found", http.StatusNotFound)
		return
	}
	var body struct {
		WorkspacePath string `json:"workspacePath"`
		Context       string `json:"context"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.WorkspacePath == "" {
		http.Error(w, "workspacePath is required", http.StatusBadRequest)
		return
	}

	var taskID string
	var err error
	switch action {
	case "score":
		taskID, err = DispatchScore(body.WorkspacePath, id, body.Context)
	case "enrich":
		taskID, err = DispatchEnrich(body.WorkspacePath, id)
	case "follow":
		taskID, err = DispatchDecision(body.WorkspacePath, id, StageContacted)
	case "drop":
		taskID, err = DispatchDecision(body.WorkspacePath, id, StageDropped)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"taskId": taskID, "action": action, "leadId": id})
}

// ── helpers ─────────────────────────────────────────────────────────────────

// tasksForLead returns inline task briefs for a lead via the reverse binding seam.
func tasksForLead(leadID string) []taskBrief {
	if liveAPI == nil {
		return []taskBrief{}
	}
	tasks, err := liveAPI.ListTasksForBusiness(LeadRef(leadID))
	if err != nil {
		return []taskBrief{}
	}
	out := make([]taskBrief, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskBrief{
			ID:       t.ID,
			Title:    t.Title,
			Status:   string(t.Status),
			Executor: string(t.Executor),
		})
	}
	return out
}

// splitLeadAction parses /api/crm/leads/{id}/{action}.
func splitLeadAction(path string) (id, action string) {
	rest := strings.TrimPrefix(path, "/api/crm/leads/")
	rest = strings.Trim(rest, "/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
