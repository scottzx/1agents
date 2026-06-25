package meta

import (
	"database/sql"
	"time"
)

// DigestTemplate is one reusable value-extraction standard: a Markdown body
// describing what counts as valuable in a chat plus the desired output shape.
// Templates are bound to chats many-to-many via digest_bindings; the ones with
// IsDefault act as the global fallback for chats with no binding.
type DigestTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scope     string    `json:"scope"` // free tag: 'global' (preset) etc.
	BodyMD    string    `json:"bodyMd"`
	Builtin   bool      `json:"builtin"`   // seeded preset (vs user-created)
	IsDefault bool      `json:"isDefault"` // part of the no-binding fallback set
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DigestStore manages digest templates and per-chat bindings (meta.db v13).
type DigestStore struct {
	db *DB
}

// NewDigestStore returns a DigestStore over db.
func NewDigestStore(db *DB) *DigestStore { return &DigestStore{db: db} }

const digestTemplateCols = `id, name, scope, body_md, builtin, is_default, created_at, updated_at`

func scanDigestTemplate(r rowScanner) (DigestTemplate, error) {
	var t DigestTemplate
	var builtin, isDefault int
	var createdAt, updatedAt string
	if err := r.Scan(&t.ID, &t.Name, &t.Scope, &t.BodyMD, &builtin, &isDefault, &createdAt, &updatedAt); err != nil {
		return DigestTemplate{}, err
	}
	t.Builtin = builtin != 0
	t.IsDefault = isDefault != 0
	t.CreatedAt = strToTime(createdAt)
	t.UpdatedAt = strToTime(updatedAt)
	return t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UpsertTemplate inserts or replaces a template by id (used for seeding presets
// and for edits). CreatedAt is preserved on update.
func (s *DigestStore) UpsertTemplate(t DigestTemplate) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	_, err := s.db.sql.Exec(`INSERT INTO digest_templates
        (`+digestTemplateCols+`)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            name = excluded.name, scope = excluded.scope, body_md = excluded.body_md,
            builtin = excluded.builtin, is_default = excluded.is_default,
            updated_at = excluded.updated_at`,
		t.ID, t.Name, t.Scope, t.BodyMD, boolToInt(t.Builtin), boolToInt(t.IsDefault),
		timeToStr(t.CreatedAt), timeToStr(t.UpdatedAt))
	return err
}

// CreateTemplate inserts a new user-defined template with a fresh id.
func (s *DigestStore) CreateTemplate(name, scope, bodyMD string, isDefault bool) (DigestTemplate, error) {
	t := DigestTemplate{
		ID:        newID(),
		Name:      name,
		Scope:     scope,
		BodyMD:    bodyMD,
		Builtin:   false,
		IsDefault: isDefault,
	}
	if t.Scope == "" {
		t.Scope = "global"
	}
	if err := s.UpsertTemplate(t); err != nil {
		return DigestTemplate{}, err
	}
	return s.getTemplate(t.ID)
}

func (s *DigestStore) getTemplate(id string) (DigestTemplate, error) {
	row := s.db.sql.QueryRow(`SELECT `+digestTemplateCols+` FROM digest_templates WHERE id = ?`, id)
	return scanDigestTemplate(row)
}

// GetTemplate returns a template by id; ok is false when absent.
func (s *DigestStore) GetTemplate(id string) (DigestTemplate, bool, error) {
	t, err := s.getTemplate(id)
	if err == sql.ErrNoRows {
		return DigestTemplate{}, false, nil
	}
	if err != nil {
		return DigestTemplate{}, false, err
	}
	return t, true, nil
}

// ListTemplates returns all templates ordered by name.
func (s *DigestStore) ListTemplates() ([]DigestTemplate, error) {
	rows, err := s.db.sql.Query(`SELECT ` + digestTemplateCols + ` FROM digest_templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DigestTemplate{}
	for rows.Next() {
		t, err := scanDigestTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTemplateBody edits a template's Markdown body. Hot: takes effect on the
// next analysis without any rebuild. Returns ErrNotFound when the id is unknown.
func (s *DigestStore) UpdateTemplateBody(id, bodyMD string) error {
	res, err := s.db.sql.Exec(`UPDATE digest_templates SET body_md = ?, updated_at = ? WHERE id = ?`,
		bodyMD, timeToStr(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTemplate removes a template and any bindings referencing it.
func (s *DigestStore) DeleteTemplate(id string) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM digest_bindings WHERE template_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM digest_templates WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// BindTemplate attaches a template to a chat session (idempotent).
func (s *DigestStore) BindTemplate(sessionID, templateID string) error {
	_, err := s.db.sql.Exec(`INSERT OR IGNORE INTO digest_bindings (session_id, template_id, created_at)
        VALUES (?, ?, ?)`, sessionID, templateID, timeToStr(time.Now().UTC()))
	return err
}

// UnbindTemplate detaches a template from a chat session.
func (s *DigestStore) UnbindTemplate(sessionID, templateID string) error {
	_, err := s.db.sql.Exec(`DELETE FROM digest_bindings WHERE session_id = ? AND template_id = ?`,
		sessionID, templateID)
	return err
}

// TemplatesForSession resolves the value standards for a chat: its explicitly
// bound templates, or — when it has none — the global default set (IsDefault).
// Results are ordered by name for stable output. This is the storage-level
// precedence; a future 群类型默认 tier would slot between the two (deferred:
// there is no group-type metadata source yet).
func (s *DigestStore) TemplatesForSession(sessionID string) ([]DigestTemplate, error) {
	rows, err := s.db.sql.Query(`SELECT `+prefixed("t", digestTemplateCols)+`
        FROM digest_bindings b JOIN digest_templates t ON t.id = b.template_id
        WHERE b.session_id = ? ORDER BY t.name`, sessionID)
	if err != nil {
		return nil, err
	}
	bound := []DigestTemplate{}
	for rows.Next() {
		t, err := scanDigestTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		bound = append(bound, t)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(bound) > 0 {
		return bound, nil
	}
	// Fallback: the global default set.
	drows, err := s.db.sql.Query(`SELECT ` + digestTemplateCols + ` FROM digest_templates WHERE is_default = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	out := []DigestTemplate{}
	for drows.Next() {
		t, err := scanDigestTemplate(drows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, drows.Err()
}

// prefixed rewrites a comma column list "a, b" into "p.a, p.b" for joins.
func prefixed(p, cols string) string {
	out := ""
	start := 0
	for i := 0; i <= len(cols); i++ {
		if i == len(cols) || cols[i] == ',' {
			seg := cols[start:i]
			// trim leading spaces
			j := 0
			for j < len(seg) && seg[j] == ' ' {
				j++
			}
			if out != "" {
				out += ", "
			}
			out += p + "." + seg[j:]
			start = i + 1
		}
	}
	return out
}
