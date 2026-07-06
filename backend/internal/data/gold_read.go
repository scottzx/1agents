package data

import "fmt"

// gold_read.go exposes the gold (融合) layer read-only for the 数据融合 viewer.
// Fusion is a SUPERSET, not a projection: a fused row returns the FULL field-
// complete silver row (silver already preserves every source field) UNIONed with
// the gold-added identity resolution (resolved contact names, unified thread,
// cross-source channels). Nothing a source carries is dropped. Rows come back as
// the same generic RecordRow the silver viewer uses, so the 多维表格 grid renders
// them unchanged.

// GoldSummaryRow is one fused domain's rollup for the 数据融合 overview.
type GoldSummaryRow struct {
	Domain      string `json:"domain"`
	Count       int    `json:"count"`
	LastUpdated int64  `json:"lastUpdated"`
}

var goldDomains = []string{"contacts", "messages", "events", "todos"}

func isGoldDomain(d string) bool {
	for _, x := range goldDomains {
		if x == d {
			return true
		}
	}
	return false
}

// GoldSummary counts each fused domain (联系人/消息/日历) for the overview cards.
func (s *Store) GoldSummary() ([]GoldSummaryRow, error) {
	specs := []struct {
		domain, query string
	}{
		{"contacts", "SELECT COUNT(*), 0 FROM contacts"},
		{"messages", "SELECT COUNT(*), COALESCE(MAX(sent_at), 0) FROM messages"},
		{"events", "SELECT COUNT(*), COALESCE(MAX(starts_at), 0) FROM calendar_events"},
		{"todos", "SELECT COUNT(*), COALESCE(MAX(due_at), 0) FROM todos"},
	}
	out := []GoldSummaryRow{}
	for _, sp := range specs {
		var cnt int
		var last int64
		if err := s.sql.QueryRow(sp.query).Scan(&cnt, &last); err != nil {
			return nil, err
		}
		if cnt == 0 {
			continue
		}
		out = append(out, GoldSummaryRow{Domain: sp.domain, Count: cnt, LastUpdated: last})
	}
	return out, nil
}

// silverTableFor resolves the physical silver table backing (domain, source) via
// the silver registry, so the fused read can pull a row's full source fields.
func silverTableFor(domain, source string) string {
	for _, d := range silverRegistry() {
		if d.Domain == domain && d.Source == source {
			return d.Table
		}
	}
	return ""
}

// silverRowFields reads one silver row (SELECT *) as generic fields, so a fused
// row can carry every field its source preserved. prefix namespaces the keys when
// unioning multiple sources into one entity (a merged person). Also returns the
// raw column map for preview extraction. Missing row → empty (a gold entity may
// outlive a silver row that a later sync deleted).
func (s *Store) silverRowFields(table, externalID, prefix string) ([]Field, map[string]string, error) {
	if table == "" {
		return nil, nil, nil
	}
	rows, err := s.sql.Query("SELECT * FROM "+table+" WHERE external_id = ? LIMIT 1", externalID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	if !rows.Next() {
		return nil, nil, rows.Err()
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, nil, err
	}
	fields := make([]Field, 0, len(cols))
	m := make(map[string]string, len(cols))
	for i, c := range cols {
		v := toStr(vals[i])
		m[c] = v
		if c == "updated_at" || c == "deleted" { // carried by the RecordRow envelope
			continue
		}
		fields = append(fields, Field{Key: prefix + c, Value: capValue(v)})
	}
	return fields, m, nil
}

// ListGold returns one fused domain's rows as generic grid rows, newest first.
func (s *Store) ListGold(domain string, limit int) ([]RecordRow, error) {
	if !isGoldDomain(domain) {
		return nil, fmt.Errorf("data: unknown gold domain %q", domain)
	}
	if limit <= 0 {
		limit = 1000
	}
	switch domain {
	case "contacts":
		return s.listGoldContacts(limit)
	case "messages":
		return s.listGoldMessages(limit)
	case "todos":
		return s.listGoldTodos(limit)
	default:
		return s.listGoldEvents(limit)
	}
}

// listGoldContacts — one row per canonical person: the gold identity (name,
// degree, all channels) UNIONed with every contributing source's full silver
// fields (iCloud birthday/nickname/addresses/…, feishu tenant_key/chat_ids/…),
// each namespaced by platform so nothing collides or is lost.
func (s *Store) listGoldContacts(limit int) ([]RecordRow, error) {
	rows, err := s.sql.Query(`SELECT id, name, degree, phone, company, title, note, tags
        FROM contacts ORDER BY degree ASC, name LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type contact struct {
		id, name, phone, company, title, note, tags string
		degree                                      int
	}
	list := []contact{}
	for rows.Next() {
		var c contact
		if err := rows.Scan(&c.id, &c.name, &c.degree, &c.phone, &c.company, &c.title, &c.note, &c.tags); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []RecordRow{}
	for _, c := range list {
		fields := []Field{
			{Key: "name", Value: c.name},
			{Key: "degree", Value: fmt.Sprintf("%d", c.degree)},
			{Key: "phone", Value: c.phone},
			{Key: "company", Value: c.company},
			{Key: "title", Value: c.title},
		}
		if c.note != "" {
			fields = append(fields, Field{Key: "note", Value: capValue(c.note)})
		}
		if c.tags != "" {
			fields = append(fields, Field{Key: "tags", Value: c.tags})
		}
		// Every identity channel, and — for the sources that keep a rich contact
		// row (iCloud, feishu users) — that row's full field set, unioned in.
		chRows, err := s.sql.Query(`SELECT platform, address FROM contact_channels
            WHERE contact_id = ? ORDER BY platform`, c.id)
		if err != nil {
			return nil, err
		}
		var channels []string
		type chan1 struct{ platform, address string }
		chs := []chan1{}
		for chRows.Next() {
			var ch chan1
			if err := chRows.Scan(&ch.platform, &ch.address); err != nil {
				chRows.Close()
				return nil, err
			}
			channels = append(channels, ch.platform+":"+ch.address)
			chs = append(chs, ch)
		}
		chRows.Close()
		fields = append(fields, Field{Key: "channels", Value: capValue(joinStrings(channels, "; "))})
		for _, ch := range chs {
			table := silverTableFor("contacts", ch.platform)
			if table == "" {
				continue // email/phone channels have no dedicated silver contact row
			}
			sf, _, err := s.silverRowFields(table, ch.address, ch.platform+".")
			if err != nil {
				return nil, err
			}
			fields = append(fields, sf...)
		}
		out = append(out, RecordRow{UID: c.id, Collection: "contact", Fields: fields, Preview: c.name})
	}
	return out, nil
}

// listGoldMessages — one row per fused message: the full source silver row
// (feishu mentions/reply-chain, MS web_link/is_read/conversation, AgentMail dir/
// attachments) plus the gold-resolved sender name + unified thread title.
func (s *Store) listGoldMessages(limit int) ([]RecordRow, error) {
	rows, err := s.sql.Query(`SELECT m.source, m.external_id, m.sent_at,
        COALESCE(sc.name, '') AS sender_name, COALESCE(t.title, '') AS thread_title
        FROM messages m
        LEFT JOIN contacts sc ON sc.id = m.sender_contact_id
        LEFT JOIN threads t ON t.id = m.thread_id
        ORDER BY m.sent_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type msg struct {
		source, externalID, sender, thread string
		sentAt                             int64
	}
	list := []msg{}
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.source, &m.externalID, &m.sentAt, &m.sender, &m.thread); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []RecordRow{}
	for _, m := range list {
		sf, raw, err := s.silverRowFields(silverTableFor("messages", m.source), m.externalID, "")
		if err != nil {
			return nil, err
		}
		fields := []Field{
			{Key: "sender", Value: m.sender},
			{Key: "thread", Value: m.thread},
		}
		fields = append(fields, sf...)
		out = append(out, RecordRow{
			UID: m.source + ":" + m.externalID, Collection: m.source, FetchedAt: m.sentAt,
			Fields: fields, Preview: capValue(messagePreview(raw)),
		})
	}
	return out, nil
}

// listGoldEvents — one row per fused event: the full source silver row (feishu
// description/meeting_url/reminders/rsvp/…, MS body/web_link/attendees) plus the
// gold-resolved organizer name, attendee count, and same-meeting fingerprint.
func (s *Store) listGoldEvents(limit int) ([]RecordRow, error) {
	rows, err := s.sql.Query(`SELECT e.source, e.external_id, e.starts_at, e.fingerprint,
        COALESCE(oc.name, '') AS organizer_name,
        (SELECT COUNT(*) FROM event_attendees a WHERE a.event_id = e.id) AS attendees
        FROM calendar_events e
        LEFT JOIN contacts oc ON oc.id = e.organizer_contact_id
        ORDER BY e.starts_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type ev struct {
		source, externalID, fingerprint, organizer string
		startsAt                                   int64
		attendees                                  int
	}
	list := []ev{}
	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.source, &e.externalID, &e.startsAt, &e.fingerprint, &e.organizer, &e.attendees); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []RecordRow{}
	for _, e := range list {
		sf, raw, err := s.silverRowFields(silverTableFor("events", e.source), e.externalID, "")
		if err != nil {
			return nil, err
		}
		fields := []Field{
			{Key: "organizer", Value: e.organizer},
			{Key: "attendees", Value: fmt.Sprintf("%d", e.attendees)},
			{Key: "fingerprint", Value: e.fingerprint},
		}
		fields = append(fields, sf...)
		out = append(out, RecordRow{
			UID: e.source + ":" + e.externalID, Collection: e.source, FetchedAt: e.startsAt,
			Fields: fields, Preview: raw["subject"],
		})
	}
	return out, nil
}

// listGoldTodos — one row per fused to-do: the full source silver row (MS
// categories/checklist/recurrence/reminder…) plus the gold-only linked_task_id
// (promote-to-agent hook). Single-source today, but reads the same way as the
// cross-source domains.
func (s *Store) listGoldTodos(limit int) ([]RecordRow, error) {
	rows, err := s.sql.Query(`SELECT source, external_id, due_at, linked_task_id
        FROM todos ORDER BY due_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type todo struct {
		source, externalID, linkedTask string
		dueAt                          int64
	}
	list := []todo{}
	for rows.Next() {
		var td todo
		if err := rows.Scan(&td.source, &td.externalID, &td.dueAt, &td.linkedTask); err != nil {
			return nil, err
		}
		list = append(list, td)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []RecordRow{}
	for _, td := range list {
		sf, raw, err := s.silverRowFields(silverTableFor("todos", td.source), td.externalID, "")
		if err != nil {
			return nil, err
		}
		fields := []Field{}
		if td.linkedTask != "" {
			fields = append(fields, Field{Key: "linked_task", Value: td.linkedTask})
		}
		fields = append(fields, sf...)
		out = append(out, RecordRow{
			UID: td.source + ":" + td.externalID, Collection: td.source, FetchedAt: td.dueAt,
			Fields: fields, Preview: raw["title"],
		})
	}
	return out, nil
}

func messagePreview(m map[string]string) string {
	for _, c := range []string{"subject", "body_text", "snippet", "body_preview"} {
		if v := m[c]; v != "" {
			return v
		}
	}
	return ""
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
