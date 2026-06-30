package crm

import (
	"regexp"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Source aggregation (#340): turn raw intake into structured contacts/leads.
//
// REUSE SEAM — the platform already runs an Inbox intake layer
// (backend/internal/meta/inbox.go: InboxStore, source-agnostic: 飞书/微信 = "im",
// email = "email", manual, rss). CRM consumes those items rather than building
// its own collector. We read via meta.InboxStore (shared meta.db) and map each
// item → crm_contact (+ optional crm_lead). The business-card parse path (#340)
// extracts Contact fields from card text (an agent/function task could OCR an
// image upstream and hand us the text; here we parse the text deterministically).

// IngestFromInbox reads non-archived inbox items and maps any that look like a
// contact (have an email or card-like content) into crm_contact. Returns the
// number of contacts created. Idempotent-ish for Phase 1: callers archive the
// inbox item after intake to avoid re-import (left to the HTTP layer).
//
// This is the documented seam #340 calls for: if the Inbox API weren't callable
// from this dir we'd define crm.IngestFromInbox(items) over an injected slice —
// but it IS callable (shared meta.db), so we read directly.
func (s *Store) IngestFromInbox(inbox *meta.InboxStore) (int, error) {
	items, err := inbox.List(false)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range items {
		c, ok := contactFromInbox(it)
		if !ok {
			continue
		}
		if _, err := s.UpsertContact(c); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// contactFromInbox maps an inbox item to a Contact. Returns ok=false when the
// item carries no usable contact signal (no email and no card-style body).
func contactFromInbox(it meta.InboxItem) (Contact, bool) {
	parsed, ok := ParseBusinessCard(it.Content)
	if !ok {
		// Fall back to title/summary as a name if an email is present anywhere.
		email := firstEmail(it.Title + " " + it.Content + " " + it.Summary)
		if email == "" {
			return Contact{}, false
		}
		parsed = Contact{Name: strings.TrimSpace(it.Title), Email: email}
	}
	parsed.Source = mapInboxSource(it.Source)
	if parsed.Name == "" {
		parsed.Name = strings.TrimSpace(it.Title)
	}
	if parsed.Name == "" {
		return Contact{}, false
	}
	return parsed, true
}

// mapInboxSource collapses an inbox source onto the CRM contact source vocabulary.
func mapInboxSource(s string) string {
	switch s {
	case meta.InboxSourceIM:
		return "im"
	case meta.InboxSourceEmail:
		return "email"
	case meta.InboxSourceManual:
		return "manual"
	default:
		return "inbox"
	}
}

// ── business-card parse (#340) ───────────────────────────────────────────────

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRe = regexp.MustCompile(`(\+?\d[\d\s\-]{6,}\d)`)
)

// ParseBusinessCard extracts Contact fields from card text (the output of an
// upstream OCR/agent task, or pasted card text). It is deterministic (token≈0):
// the first non-empty line is the name; recognised keyword lines fill company /
// title; email & phone are regex-extracted from anywhere. ok=false when no email
// AND no phone are found (too weak to be a contact).
func ParseBusinessCard(text string) (Contact, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Contact{}, false
	}
	email := firstEmail(text)
	phone := firstPhone(text)
	if email == "" && phone == "" {
		return Contact{}, false
	}
	var c Contact
	c.Email = email
	c.Phone = phone
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case c.Name == "" && !emailRe.MatchString(line) && !phoneRe.MatchString(line):
			c.Name = line
		case hasAny(lower, "公司", "company", "inc", "ltd", "co.", "corp"):
			c.Company = stripLabel(line)
		case hasAny(lower, "title", "职位", "ceo", "cto", "manager", "经理", "总监", "director"):
			c.Title = stripLabel(line)
		}
	}
	c.Source = "card"
	return c, true
}

func firstEmail(s string) string { return emailRe.FindString(s) }

func firstPhone(s string) string {
	m := phoneRe.FindString(s)
	return strings.TrimSpace(m)
}

func hasAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// stripLabel removes a leading "key:" / "key：" prefix from a card line.
func stripLabel(line string) string {
	for _, sep := range []string{":", "："} {
		if i := strings.Index(line, sep); i >= 0 && i < 12 {
			return strings.TrimSpace(line[i+len(sep):])
		}
	}
	return line
}
