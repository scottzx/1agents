// Package crm is the CRM installable app (Epic #317 Phase 1, issues #339-343).
//
// CRM is a global L1-page application: it sinks contacts and mines leads across
// projects. It is ADDITIVE — it owns only its own crm_* domain tables, calls the
// North Task API for all AI work, and never touches core tables or the task main
// flow.
//
// The id "crm" is the single namespace threaded through everything:
//   - business_ref prefix → "crm:lead:42" / "crm:contact:7"
//   - domain table prefix → crm_contact / crm_lead
//   - taskType / function → "crm.enrich"
//   - RegisterApp Namespace → "crm"
package crm

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Lead stage constants — the funnel columns (#343).
const (
	StageNew       = "new"
	StageContacted = "contacted"
	StageQualified = "qualified"
	StageWon       = "won"
	StageLost      = "lost"
	StageDropped   = "dropped"
)

// Contact is a sunk contact (沉淀). Mirrors crm_contact.
type Contact struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Company   string    `json:"company,omitempty"`
	Title     string    `json:"title,omitempty"`
	Email     string    `json:"email,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	Source    string    `json:"source,omitempty"` // im / email / manual / card / inbox
	CreatedAt time.Time `json:"createdAt"`
}

// Lead is a mined opportunity (挖掘). Mirrors crm_lead.
type Lead struct {
	ID          string    `json:"id"`
	ContactID   string    `json:"contactId"`
	Stage       string    `json:"stage"`
	Score       int       `json:"score"`
	Owner       string    `json:"owner,omitempty"`
	BusinessRef string    `json:"businessRef,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Store is the CRM domain store over the shared meta.db connection. It owns only
// crm_* tables; it never reads or writes core tables.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store over the given *sql.DB (meta.db).
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ── Contacts ────────────────────────────────────────────────────────────────

// UpsertContact inserts a contact (assigning an id when empty) and returns it.
// Dedup is left to callers for Phase 1 (single user, honour-based).
func (s *Store) UpsertContact(c Contact) (Contact, error) {
	if c.ID == "" {
		c.ID = meta.NewID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
		INSERT INTO crm_contact (id, name, company, title, email, phone, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, company=excluded.company, title=excluded.title,
			email=excluded.email, phone=excluded.phone, source=excluded.source`,
		c.ID, c.Name, c.Company, c.Title, c.Email, c.Phone, c.Source, timeStr(c.CreatedAt))
	if err != nil {
		return Contact{}, fmt.Errorf("crm: upsert contact: %w", err)
	}
	return c, nil
}

// ListContacts returns all contacts newest-first.
func (s *Store) ListContacts() ([]Contact, error) {
	rows, err := s.db.Query(`SELECT id, name, company, title, email, phone, source, created_at
		FROM crm_contact ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contact{}
	for rows.Next() {
		var c Contact
		var created string
		if err := rows.Scan(&c.ID, &c.Name, &c.Company, &c.Title, &c.Email, &c.Phone, &c.Source, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetContact returns a contact by id; ok=false when absent.
func (s *Store) GetContact(id string) (Contact, bool, error) {
	row := s.db.QueryRow(`SELECT id, name, company, title, email, phone, source, created_at
		FROM crm_contact WHERE id = ?`, id)
	var c Contact
	var created string
	err := row.Scan(&c.ID, &c.Name, &c.Company, &c.Title, &c.Email, &c.Phone, &c.Source, &created)
	if err == sql.ErrNoRows {
		return Contact{}, false, nil
	}
	if err != nil {
		return Contact{}, false, err
	}
	c.CreatedAt = parseTime(created)
	return c, true, nil
}

// ── Leads ───────────────────────────────────────────────────────────────────

// CreateLead inserts a new lead, assigning id, business_ref, and timestamps.
// Stage defaults to "new". business_ref is set to "crm:lead:<id>".
func (s *Store) CreateLead(l Lead) (Lead, error) {
	if l.ID == "" {
		l.ID = meta.NewID()
	}
	if l.Stage == "" {
		l.Stage = StageNew
	}
	l.BusinessRef = LeadRef(l.ID)
	now := time.Now().UTC()
	if l.CreatedAt.IsZero() {
		l.CreatedAt = now
	}
	l.UpdatedAt = now
	_, err := s.db.Exec(`
		INSERT INTO crm_lead (id, contact_id, stage, score, owner, business_ref, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.ContactID, l.Stage, l.Score, l.Owner, l.BusinessRef, l.Notes,
		timeStr(l.CreatedAt), timeStr(l.UpdatedAt))
	if err != nil {
		return Lead{}, fmt.Errorf("crm: create lead: %w", err)
	}
	return l, nil
}

// ListLeads returns all leads newest-first.
func (s *Store) ListLeads() ([]Lead, error) {
	rows, err := s.db.Query(`SELECT id, contact_id, stage, score, owner, business_ref, notes, created_at, updated_at
		FROM crm_lead ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lead{}
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetLead returns a lead by id; ok=false when absent.
func (s *Store) GetLead(id string) (Lead, bool, error) {
	row := s.db.QueryRow(`SELECT id, contact_id, stage, score, owner, business_ref, notes, created_at, updated_at
		FROM crm_lead WHERE id = ?`, id)
	l, err := scanLead(row)
	if err == sql.ErrNoRows {
		return Lead{}, false, nil
	}
	if err != nil {
		return Lead{}, false, err
	}
	return l, true, nil
}

// UpdateLeadStage advances a lead's stage and bumps updated_at. Returns
// ErrNotFound-style ok=false when the lead is unknown.
func (s *Store) UpdateLeadStage(id, stage string) (bool, error) {
	res, err := s.db.Exec(`UPDATE crm_lead SET stage = ?, updated_at = ? WHERE id = ?`,
		stage, timeStr(time.Now().UTC()), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateLeadScore sets a lead's score (and optionally notes) and bumps updated_at.
// notes is appended only when non-empty. Returns ok=false when the lead is unknown.
func (s *Store) UpdateLeadScore(id string, score int, notes string) (bool, error) {
	query := `UPDATE crm_lead SET score = ?, updated_at = ?`
	args := []any{score, timeStr(time.Now().UTC())}
	if notes != "" {
		query += `, notes = ?`
		args = append(args, notes)
	}
	query += ` WHERE id = ?`
	args = append(args, id)
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

// LeadRef returns the canonical business_ref for a lead id.
func LeadRef(id string) string { return "crm:lead:" + id }

// ContactRef returns the canonical business_ref for a contact id.
func ContactRef(id string) string { return "crm:contact:" + id }

// LeadIDFromRef extracts the lead id from a "crm:lead:<id>" ref; ok=false otherwise.
func LeadIDFromRef(ref string) (string, bool) {
	const prefix = "crm:lead:"
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	return strings.TrimPrefix(ref, prefix), true
}

func scanLead(r interface{ Scan(...any) error }) (Lead, error) {
	var l Lead
	var created, updated string
	if err := r.Scan(&l.ID, &l.ContactID, &l.Stage, &l.Score, &l.Owner,
		&l.BusinessRef, &l.Notes, &created, &updated); err != nil {
		return Lead{}, err
	}
	l.CreatedAt = parseTime(created)
	l.UpdatedAt = parseTime(updated)
	return l, nil
}

func timeStr(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}
