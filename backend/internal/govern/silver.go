package govern

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// silver.go is the bronze→silver stage: per-SOURCE, source-faithful cleaning.
// Each source's governor knows its own raw shape and writes its own native
// silver table (internal/data), preserving everything valuable — Apple
// birthday/nickname/note, 飞书 @mentions + reply chain, MS todo
// recurrence/checklist. It is re-runnable: a per-(source,kind) StageSilver cursor
// over bronze.fetched_at tracks progress, and every upsert is idempotent, so
// resetting the cursor re-shapes everything without a network round-trip.
// Cross-source unification is gold (step 3), not here.

// Bronze kind identifiers (declared where the pullers live; the catalog uses the
// same strings).
const (
	kindFeishuMessage = "feishu_message"
	kindFeishuChat    = "feishu_chat"
	kindMSMail        = "ms_mail"
	kindAgentMail     = "agentmail_mail"
	kindMSEvent       = "ms_event"
	kindMSTodo        = "ms_todo"
)

// SilverStats reports how many silver rows each viewer domain wrote in a run
// (contacts = icloud + 飞书二级用户; messages = 飞书 + MS + AgentMail).
type SilverStats struct {
	Contacts, Messages, Events, Todos int
}

// Silver runs every source's bronze→silver transform.
func Silver(src *sources.Store, dst *data.Store) (SilverStats, error) {
	var st SilverStats
	steps := []struct {
		add *int
		run func(*sources.Store, *data.Store) (int, error)
	}{
		{&st.Contacts, SilverIcloudContacts},
		{&st.Contacts, SilverFeishuUsers},
		{&st.Messages, SilverFeishuMessages},
		{&st.Messages, SilverMicrosoftMail},
		{&st.Messages, SilverAgentMail},
		{&st.Events, SilverMicrosoftEvents},
		{&st.Todos, SilverMicrosoftTodos},
		{nil, SilverFeishuChats}, // group metadata; supports gold threads, not a viewer domain
	}
	for _, s := range steps {
		n, err := s.run(src, dst)
		if err != nil {
			return st, err
		}
		if s.add != nil {
			*s.add += n
		}
	}
	return st, nil
}

// ---- 1:1 governors (one bronze record → one silver row), via runSilver ----

func SilverIcloudContacts(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.SourceICloud, sources.KindContact, parseIcloudContact, dst.UpsertIcloudContacts)
}
func SilverFeishuMessages(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorFeishu, kindFeishuMessage, parseFeishuMessage, dst.UpsertFeishuMessages)
}
func SilverFeishuChats(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorFeishu, kindFeishuChat, parseFeishuChat, dst.UpsertFeishuChats)
}
func SilverMicrosoftMail(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorMicrosoft, kindMSMail, parseMicrosoftMail, dst.UpsertMicrosoftMail)
}
func SilverAgentMail(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorAgentMail, kindAgentMail, parseAgentMail, dst.UpsertAgentMail)
}
func SilverMicrosoftEvents(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorMicrosoft, kindMSEvent, parseMicrosoftEvent, dst.UpsertMicrosoftEvents)
}
func SilverMicrosoftTodos(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorMicrosoft, kindMSTodo, parseMicrosoftTodo, dst.UpsertMicrosoftTodos)
}

// runSilver drives one (source, kind) feed: read the bronze slice since the
// StageSilver cursor, parse each record into zero-or-more silver rows, upsert
// them, and advance the cursor to the max fetched_at seen.
func runSilver[T any](src *sources.Store, dst *data.Store, source, kind string,
	parse func(sources.StoredRecord) []T, upsert func([]T) (int, error)) (int, error) {
	since, err := dst.GovernCursor(data.StageSilver, source, kind)
	if err != nil {
		return 0, err
	}
	recs, maxFetched, err := src.RecordsSince(source, kind, since)
	if err != nil {
		return 0, err
	}
	var rows []T
	for _, r := range recs {
		rows = append(rows, parse(r)...)
	}
	n, err := upsert(rows)
	if err != nil {
		return 0, err
	}
	if err := dst.SaveGovernCursor(data.StageSilver, source, kind, maxFetched); err != nil {
		return n, err
	}
	return n, nil
}

// ---- 飞书 二级用户 aggregate governor ----

// SilverFeishuUsers rebuilds the 二级用户 table from EVERY 飞书 message's sender +
// @mentions (name comes free from mentions[].name). It is a full recompute over
// the message stream each run (not cursor-gated): cheap at chat scale and always
// consistent. Once a feishu_user bronze kind (group roster) lands, it merges in
// as a second discovery source.
func SilverFeishuUsers(src *sources.Store, dst *data.Store) (int, error) {
	recs, maxFetched, err := src.RecordsSince(sources.VendorFeishu, kindFeishuMessage, 0)
	if err != nil {
		return 0, err
	}
	users := map[string]*data.SilverFeishuUser{}
	touch := func(openID, name, tenant, via, chatID string, t int64) {
		if openID == "" {
			return
		}
		u := users[openID]
		if u == nil {
			u = &data.SilverFeishuUser{ExternalID: openID, UpdatedAt: maxFetched}
			users[openID] = u
		}
		if u.Name == "" && name != "" {
			u.Name = name
		}
		if u.TenantKey == "" && tenant != "" {
			u.TenantKey = tenant
		}
		u.DiscoveredVia = addUnique(u.DiscoveredVia, via)
		u.ChatIDs = addUnique(u.ChatIDs, chatID)
		if via == "sender" {
			u.AsSenderCount++
		}
		if t > 0 {
			if u.FirstSeen == 0 || t < u.FirstSeen {
				u.FirstSeen = t
			}
			if t > u.LastSeen {
				u.LastSeen = t
			}
		}
	}
	for _, r := range recs {
		m := decodeFeishuMessage(r.Payload)
		if m == nil {
			continue
		}
		t := parseEpochMS(m.CreateTime)
		touch(m.Sender.ID, "", m.Sender.TenantKey, "sender", m.ChatID, t)
		for _, mn := range m.Mentions {
			touch(mn.ID, mn.Name, mn.TenantKey, "mention", m.ChatID, t)
		}
	}
	rows := make([]data.SilverFeishuUser, 0, len(users))
	for _, u := range users {
		rows = append(rows, *u)
	}
	n, err := dst.UpsertFeishuUsers(rows)
	if err != nil {
		return 0, err
	}
	// A dedicated cursor slot so a future incremental version can resume.
	_ = dst.SaveGovernCursor(data.StageSilver, sources.VendorFeishu, "feishu_user", maxFetched)
	return n, nil
}

func addUnique(xs []string, v string) []string {
	if v == "" {
		return xs
	}
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

// ---- shared payload fragments & time helpers ----

type graphEmail struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type graphDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

var isoLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.9999999Z07:00",
	"2006-01-02T15:04:05.9999999", // Graph often omits the zone (implicit UTC)
	"2006-01-02T15:04:05",
}

func parseISOTime(s string) int64 {
	if s == "" {
		return 0
	}
	for _, l := range isoLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func parseGraphDT(g *graphDateTime) int64 {
	if g == nil {
		return 0
	}
	return parseISOTime(g.DateTime)
}

func parseEpochMS(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string { return strings.TrimSpace(tagRe.ReplaceAllString(s, "")) }

// ---- iCloud vCard (lossless) ----

func parseIcloudContact(r sources.StoredRecord) []data.SilverIcloudContact {
	cards := splitVCards(r.Payload)
	out := make([]data.SilverIcloudContact, 0, len(cards))
	for i, card := range cards {
		ext := r.UID
		if len(cards) > 1 {
			ext = fmt.Sprintf("%s#%d", r.UID, i)
		}
		c := data.SilverIcloudContact{AccountID: r.AccountID, ExternalID: ext, Deleted: r.Deleted, UpdatedAt: r.FetchedAt}
		for _, p := range icloud.VCardProps(card) {
			key, val := p[0], strings.TrimSpace(p[1])
			switch key {
			case "FN":
				c.FullName = val
			case "N":
				parts := strings.Split(val, ";")
				if len(parts) > 0 {
					c.FamilyName = parts[0]
				}
				if len(parts) > 1 {
					c.GivenName = parts[1]
				}
			case "TEL":
				if val != "" {
					c.Phones = append(c.Phones, val)
				}
			case "EMAIL":
				if val != "" {
					c.Emails = append(c.Emails, val)
				}
			case "ORG":
				c.Org = strings.SplitN(val, ";", 2)[0]
			case "TITLE":
				c.Title = val
			case "BDAY":
				c.Birthday = val
			case "NICKNAME":
				c.Nickname = val
			case "NOTE":
				c.Note = val
			case "IMPP":
				if val != "" {
					c.IMHandles = append(c.IMHandles, val)
				}
			case "URL":
				if val != "" {
					c.URLs = append(c.URLs, val)
				}
			case "ADR":
				if val != "" {
					c.Addresses = append(c.Addresses, val)
				}
			}
		}
		out = append(out, c)
	}
	return out
}

// splitVCards splits a payload into individual BEGIN:VCARD..END:VCARD blocks so
// each card is a distinct silver row (a resource is usually one card).
func splitVCards(payload string) []string {
	lines := strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n")
	var cards []string
	var cur []string
	in := false
	for _, l := range lines {
		u := strings.ToUpper(strings.TrimSpace(l))
		if strings.HasPrefix(u, "BEGIN:VCARD") {
			in, cur = true, nil
		}
		if in {
			cur = append(cur, l)
		}
		if strings.HasPrefix(u, "END:VCARD") {
			in = false
			cards = append(cards, strings.Join(cur, "\n"))
		}
	}
	if len(cards) == 0 {
		return []string{payload}
	}
	return cards
}

// ---- 飞书 message + user + chat ----

type feishuMsg struct {
	Body struct {
		Content string `json:"content"`
	} `json:"body"`
	ChatID     string `json:"chat_id"`
	CreateTime string `json:"create_time"`
	Deleted    bool   `json:"deleted"`
	MessageID  string `json:"message_id"`
	MsgType    string `json:"msg_type"`
	ParentID   string `json:"parent_id"`
	RootID     string `json:"root_id"`
	ThreadID   string `json:"thread_id"`
	Sender     struct {
		ID        string `json:"id"`
		TenantKey string `json:"tenant_key"`
	} `json:"sender"`
	Mentions []struct {
		ID        string `json:"id"`
		Key       string `json:"key"`
		Name      string `json:"name"`
		TenantKey string `json:"tenant_key"`
	} `json:"mentions"`
}

func decodeFeishuMessage(payload string) *feishuMsg {
	var m feishuMsg
	if json.Unmarshal([]byte(payload), &m) != nil {
		return nil
	}
	return &m
}

func parseFeishuMessage(r sources.StoredRecord) []data.SilverFeishuMessage {
	m := decodeFeishuMessage(r.Payload)
	if m == nil {
		return nil
	}
	ext := m.MessageID
	if ext == "" {
		ext = r.UID
	}
	mentions := make([]data.Mention, 0, len(m.Mentions))
	for _, mn := range m.Mentions {
		mentions = append(mentions, data.Mention{OpenID: mn.ID, Key: mn.Key, Name: mn.Name, TenantKey: mn.TenantKey})
	}
	return []data.SilverFeishuMessage{{
		AccountID:       r.AccountID,
		ExternalID:      ext,
		ChatID:          m.ChatID,
		MsgType:         m.MsgType,
		SenderOpenID:    m.Sender.ID,
		SenderTenantKey: m.Sender.TenantKey,
		BodyText:        feishuText(m.MsgType, m.Body.Content),
		Mentions:        mentions,
		ParentID:        m.ParentID,
		RootID:          m.RootID,
		ThreadID:        m.ThreadID,
		CreateTime:      parseEpochMS(m.CreateTime),
		Deleted:         m.Deleted || r.Deleted,
		UpdatedAt:       r.FetchedAt,
	}}
}

func parseFeishuChat(r sources.StoredRecord) []data.SilverFeishuChat {
	var c struct {
		ChatID      string `json:"chat_id"`
		Name        string `json:"name"`
		ChatMode    string `json:"chat_mode"`
		External    bool   `json:"external"`
		OwnerID     string `json:"owner_id"`
		TenantKey   string `json:"tenant_key"`
		Avatar      string `json:"avatar"`
		Description string `json:"description"`
	}
	if json.Unmarshal([]byte(r.Payload), &c) != nil {
		return nil
	}
	ext := c.ChatID
	if ext == "" {
		ext = r.UID
	}
	return []data.SilverFeishuChat{{
		AccountID:   r.AccountID,
		ExternalID:  ext,
		Name:        c.Name,
		ChatMode:    c.ChatMode,
		External:    c.External,
		OwnerOpenID: c.OwnerID,
		TenantKey:   c.TenantKey,
		Avatar:      c.Avatar,
		Description: c.Description,
		Deleted:     r.Deleted,
		UpdatedAt:   r.FetchedAt,
	}}
}

// feishuText extracts plain text from a 飞书 body.content per msg_type; non-textual
// types degrade to "[msg_type]".
func feishuText(msgType, content string) string {
	switch msgType {
	case "text":
		var t struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &t) == nil {
			return stripTags(t.Text)
		}
	case "post":
		var p struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag  string `json:"tag"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal([]byte(content), &p) == nil {
			var b strings.Builder
			if p.Title != "" {
				b.WriteString(p.Title)
				b.WriteByte(' ')
			}
			for _, line := range p.Content {
				for _, seg := range line {
					b.WriteString(seg.Text)
				}
				b.WriteByte(' ')
			}
			if s := strings.TrimSpace(b.String()); s != "" {
				return s
			}
		}
	}
	return "[" + msgType + "]"
}

// ---- Microsoft mail / event / todo + AgentMail ----

func parseMicrosoftMail(r sources.StoredRecord) []data.SilverMicrosoftMail {
	var m struct {
		ID               string `json:"id"`
		Subject          string `json:"subject"`
		BodyPreview      string `json:"bodyPreview"`
		ReceivedDateTime string `json:"receivedDateTime"`
		ConversationID   string `json:"conversationId"`
		IsRead           bool   `json:"isRead"`
		WebLink          string `json:"webLink"`
		From             struct {
			EmailAddress graphEmail `json:"emailAddress"`
		} `json:"from"`
		ToRecipients []struct {
			EmailAddress graphEmail `json:"emailAddress"`
		} `json:"toRecipients"`
	}
	if json.Unmarshal([]byte(r.Payload), &m) != nil {
		return nil
	}
	to := make([]data.EmailRef, 0, len(m.ToRecipients))
	for _, t := range m.ToRecipients {
		to = append(to, data.EmailRef{Addr: t.EmailAddress.Address, Name: t.EmailAddress.Name})
	}
	ext := m.ID
	if ext == "" {
		ext = r.UID
	}
	return []data.SilverMicrosoftMail{{
		AccountID: r.AccountID, ExternalID: ext,
		Subject: m.Subject, BodyPreview: m.BodyPreview, ReceivedAt: parseISOTime(m.ReceivedDateTime),
		FromAddr: m.From.EmailAddress.Address, FromName: m.From.EmailAddress.Name,
		ToRecipients: to, IsRead: m.IsRead, WebLink: m.WebLink, ConversationID: m.ConversationID,
		Deleted: r.Deleted, UpdatedAt: r.FetchedAt,
	}}
}

func parseAgentMail(r sources.StoredRecord) []data.SilverAgentMail {
	var m struct {
		MessageID      string `json:"message_id"`
		Subject        string `json:"subject"`
		Snippet        string `json:"snippet"`
		CreatedAt      string `json:"created_at"`
		HasAttachments bool   `json:"has_attachments"`
		IsRead         bool   `json:"is_read"`
		Dir            struct {
			DirName string `json:"dir_name"`
		} `json:"dir"`
		From struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"from"`
		To []struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"to"`
	}
	if json.Unmarshal([]byte(r.Payload), &m) != nil {
		return nil
	}
	to := make([]data.EmailRef, 0, len(m.To))
	for _, t := range m.To {
		to = append(to, data.EmailRef{Addr: t.Email, Name: t.Name})
	}
	ext := m.MessageID
	if ext == "" {
		ext = r.UID
	}
	return []data.SilverAgentMail{{
		AccountID: r.AccountID, ExternalID: ext,
		Subject: m.Subject, Snippet: m.Snippet, CreatedAtSrc: parseISOTime(m.CreatedAt),
		FromEmail: m.From.Email, FromName: m.From.Name, ToRecipients: to,
		DirName: m.Dir.DirName, HasAttachments: m.HasAttachments, IsRead: m.IsRead,
		Deleted: r.Deleted, UpdatedAt: r.FetchedAt,
	}}
}

func parseMicrosoftEvent(r sources.StoredRecord) []data.SilverMicrosoftEvent {
	var e struct {
		ID       string `json:"id"`
		Subject  string `json:"subject"`
		IsAllDay bool   `json:"isAllDay"`
		ShowAs   string `json:"showAs"`
		WebLink  string `json:"webLink"`
		Body     struct {
			Content string `json:"content"`
		} `json:"body"`
		Start    graphDateTime `json:"start"`
		End      graphDateTime `json:"end"`
		Location struct {
			DisplayName string `json:"displayName"`
		} `json:"location"`
		Organizer struct {
			EmailAddress graphEmail `json:"emailAddress"`
		} `json:"organizer"`
		Attendees []struct {
			EmailAddress graphEmail `json:"emailAddress"`
			Status       struct {
				Response string `json:"response"`
			} `json:"status"`
		} `json:"attendees"`
		Recurrence json.RawMessage `json:"recurrence"`
	}
	if json.Unmarshal([]byte(r.Payload), &e) != nil {
		return nil
	}
	att := make([]data.Attendee, 0, len(e.Attendees))
	for _, a := range e.Attendees {
		att = append(att, data.Attendee{Addr: a.EmailAddress.Address, Name: a.EmailAddress.Name, Response: a.Status.Response})
	}
	ext := e.ID
	if ext == "" {
		ext = r.UID
	}
	return []data.SilverMicrosoftEvent{{
		AccountID: r.AccountID, ExternalID: ext, CalendarID: r.Collection,
		Subject: e.Subject, Body: stripTags(e.Body.Content), Location: e.Location.DisplayName,
		StartsAt: parseISOTime(e.Start.DateTime), EndsAt: parseISOTime(e.End.DateTime), AllDay: e.IsAllDay,
		ShowAs: e.ShowAs, WebLink: e.WebLink,
		OrganizerAddr: e.Organizer.EmailAddress.Address, OrganizerName: e.Organizer.EmailAddress.Name,
		Attendees: att, Recurrence: rawJSON(e.Recurrence), Deleted: r.Deleted, UpdatedAt: r.FetchedAt,
	}}
}

func parseMicrosoftTodo(r sources.StoredRecord) []data.SilverMicrosoftTodo {
	var t struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		Importance string `json:"importance"`
		IsReminder bool   `json:"isReminderOn"`
		HasAttach  bool   `json:"hasAttachments"`
		Body       struct {
			Content string `json:"content"`
		} `json:"body"`
		Created           string          `json:"createdDateTime"`
		DueDateTime       *graphDateTime  `json:"dueDateTime"`
		CompletedDateTime *graphDateTime  `json:"completedDateTime"`
		ReminderDateTime  *graphDateTime  `json:"reminderDateTime"`
		Categories        []string        `json:"categories"`
		Recurrence        json.RawMessage `json:"recurrence"`
		ChecklistItems    json.RawMessage `json:"checklistItems"`
		LinkedResources   json.RawMessage `json:"linkedResources"`
	}
	if json.Unmarshal([]byte(r.Payload), &t) != nil {
		return nil
	}
	ext := t.ID
	if ext == "" {
		ext = r.UID
	}
	return []data.SilverMicrosoftTodo{{
		AccountID: r.AccountID, ExternalID: ext, ListID: r.Collection,
		Title: t.Title, Body: t.Body.Content, Status: t.Status, Importance: t.Importance,
		DueAt: parseGraphDT(t.DueDateTime), CompletedAt: parseGraphDT(t.CompletedDateTime),
		CreatedAtSrc: parseISOTime(t.Created), ReminderAt: parseGraphDT(t.ReminderDateTime),
		IsReminderOn: t.IsReminder, HasAttachments: t.HasAttach, Categories: t.Categories,
		Recurrence: rawJSON(t.Recurrence), ChecklistItems: rawJSONArr(t.ChecklistItems),
		LinkedResources: rawJSONArr(t.LinkedResources), Deleted: r.Deleted, UpdatedAt: r.FetchedAt,
	}}
}

// rawJSON returns the compact JSON text of a raw message, "" when absent/null.
func rawJSON(m json.RawMessage) string {
	s := strings.TrimSpace(string(m))
	if s == "" || s == "null" {
		return ""
	}
	return s
}

// rawJSONArr is rawJSON but defaults to "[]" for array columns.
func rawJSONArr(m json.RawMessage) string {
	if s := rawJSON(m); s != "" {
		return s
	}
	return "[]"
}
