package data

import (
	"fmt"
	"sort"
	"strings"
)

// reader.go exposes the silver layer read-only for the 数据归一 viewer. Silver is
// per-SOURCE physical tables grouped into domains (联系人/消息/日历/待办) by a
// registry; a domain view unions its source tables. Rows come back as generic
// (key,value) fields so the same schema-free 多维表格 grid renders any source's
// native columns (Apple birthday vs 飞书 open_id) side by side.

// Field is one native column of a silver row (key = the column name).
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RecordRow mirrors sources.SourceRecordRow (same JSON shape) so the bronze
// viewer's grid renders silver unchanged. UID=external_id, Collection=source,
// FetchedAt=updated_at.
type RecordRow struct {
	UID         string  `json:"uid"`
	Collection  string  `json:"collection"`
	ETag        string  `json:"etag"`
	ContentType string  `json:"contentType"`
	Deleted     bool    `json:"deleted"`
	FetchedAt   int64   `json:"fetchedAt"`
	Fields      []Field `json:"fields"`
	Preview     string  `json:"preview"`
}

// SilverSummaryRow is one (domain, source) rollup for the overview.
type SilverSummaryRow struct {
	Domain      string `json:"domain"`
	Source      string `json:"source"`
	Count       int    `json:"count"`
	LastUpdated int64  `json:"lastUpdated"`
}

// silverTableDef maps one per-source physical silver table into a viewer domain.
type silverTableDef struct{ Domain, Source, Table string }

// silverRegistry flattens every registered source's viewer-exposed tables — the
// single source of truth for which tables belong to which domain, now assembled
// from the per-source registrations (issue #399) rather than a hardcoded literal.
// silver_feishu_chats is intentionally absent (each source chooses which of its
// tables to expose): it is group metadata for gold thread titles, not a browsable
// domain.
func silverRegistry() []silverTableDef {
	var out []silverTableDef
	for _, src := range silverSources {
		out = append(out, src.tables...)
	}
	return out
}

var domainOrder = []string{"contacts", "messages", "events", "todos"}

// previewCols is the per-domain columns tried (in order) for a row's one-line
// preview — broad enough to cover each source's differing column names.
var previewCols = map[string][]string{
	"contacts": {"full_name", "name", "org"},
	"messages": {"subject", "body_text", "snippet"},
	"events":   {"subject", "location"},
	"todos":    {"title", "body"},
}

func isDomain(d string) bool {
	for _, x := range domainOrder {
		if x == d {
			return true
		}
	}
	// Manifest sources may declare their own domain (e.g. fitness); accept any
	// domain that a registered viewer table belongs to.
	for _, t := range silverRegistry() {
		if t.Domain == d {
			return true
		}
	}
	return false
}

// ListSilver returns a domain's conformed rows across its source tables, newest
// first. source (optional) scopes to one source; limit<=0 uses a default cap.
func (s *Store) ListSilver(domain, source string, limit int) ([]RecordRow, error) {
	if !isDomain(domain) {
		return nil, fmt.Errorf("data: unknown silver domain %q", domain)
	}
	if limit <= 0 {
		limit = 1000
	}
	out := []RecordRow{}
	for _, d := range silverRegistry() {
		if d.Domain != domain || (source != "" && d.Source != source) {
			continue
		}
		rows, err := s.readTable(domain, d.Table, limit)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].FetchedAt > out[j].FetchedAt })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// readTable reads one silver table's rows (SELECT *) as generic grid rows.
func (s *Store) readTable(domain, table string, limit int) ([]RecordRow, error) {
	rows, err := s.sql.Query("SELECT * FROM "+table+" ORDER BY updated_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []RecordRow{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]string, len(cols))
		fields := make([]Field, 0, len(cols))
		for i, c := range cols {
			v := toStr(vals[i])
			m[c] = v
			if c == "updated_at" { // surfaced as FetchedAt (时间), not a duplicate field
				continue
			}
			fields = append(fields, Field{Key: c, Value: capValue(v)})
		}
		out = append(out, RecordRow{
			UID:        m["external_id"],
			Collection: m["source"],
			Deleted:    m["deleted"] == "1",
			FetchedAt:  parseInt(m["updated_at"]),
			Fields:     fields,
			Preview:    previewOf(domain, m),
		})
	}
	return out, rows.Err()
}

// SilverSummary rolls up every registered table by (domain, source).
func (s *Store) SilverSummary() ([]SilverSummaryRow, error) {
	out := []SilverSummaryRow{}
	for _, d := range silverRegistry() {
		var cnt int
		var last int64
		err := s.sql.QueryRow("SELECT COUNT(*), COALESCE(MAX(updated_at), 0) FROM "+d.Table).Scan(&cnt, &last)
		if err != nil {
			return nil, err
		}
		if cnt == 0 {
			continue
		}
		out = append(out, SilverSummaryRow{Domain: d.Domain, Source: d.Source, Count: cnt, LastUpdated: last})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return domainRank(out[i].Domain) < domainRank(out[j].Domain)
		}
		return out[i].Source < out[j].Source
	})
	return out, nil
}

func domainRank(d string) int {
	for i, x := range domainOrder {
		if x == d {
			return i
		}
	}
	return len(domainOrder)
}

func previewOf(domain string, m map[string]string) string {
	for _, c := range previewCols[domain] {
		if v := strings.TrimSpace(m[c]); v != "" {
			if len(v) > 160 {
				return v[:160]
			}
			return v
		}
	}
	return ""
}

// toStr renders a scanned SQLite value as a string (the pure-Go driver returns
// []byte for text, int64 for integers).
func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

func capValue(s string) string {
	const max = 500
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func parseInt(s string) int64 {
	var v int64
	_, _ = fmt.Sscan(s, &v)
	return v
}
