package govern

import (
	"strconv"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// gold.go is the silver→gold fusion stage (step 3, 归一). Unlike silver (which
// reads bronze), gold reads and writes data.db only: it resolves every 飞书 OpenID
// — message sender + @mention — to a canonical contact via contact_channels,
// groups messages into threads (title from silver_feishu_chats), and records
// who-sent-what / who-was-@ed as message_participants. v1 fuses only 飞书 and only
// on the deterministic OpenID key (cross-source phone/email person merge is v2).
// A StageGold cursor over silver.updated_at makes it re-runnable; every write is
// idempotent, so resetting the cursor safely re-fuses everything.

// GoldStats reports how many gold rows a fusion run wrote.
type GoldStats struct {
	Threads  int
	Messages int
	Contacts int // newly created contacts (degree-2 discovered + degree-1 address-book)
	Events   int
	Todos    int
}

// Gold runs every source's silver→gold fusion in dependency order: the iCloud
// address book seeds degree-1 contacts first (so an email/phone message sender
// folds into a known person rather than a bare degree-2), then messages, calendar
// events, then to-dos. Each governor is cursor-gated + idempotent, so the whole
// pipeline is safely re-runnable.
func Gold(dst *data.Store) (GoldStats, error) {
	var total GoldStats
	steps := []func(*data.Store) (GoldStats, error){
		GoldContactsIcloud,  // ① 人 — address-book degree-1 hub
		GoldFeishu,          // ② 消息 — 飞书 IM
		GoldMicrosoftMail,   // ② 消息 — MS 邮件
		GoldAgentMail,       // ② 消息 — AgentMail
		GoldFeishuEvents,    // ③ 日历 — 飞书事件
		GoldMicrosoftEvents, // ③ 日历 — MS 事件
		GoldMicrosoftTodos,  // ④ 待办 — MS 待办 (single-source today)
	}
	for _, step := range steps {
		st, err := step(dst)
		if err != nil {
			return total, err
		}
		total.Threads += st.Threads
		total.Messages += st.Messages
		total.Contacts += st.Contacts
		total.Events += st.Events
		total.Todos += st.Todos
	}
	return total, nil
}

// GoldFeishu fuses silver 飞书 messages into gold threads/messages/participants,
// resolving each sender + @mention OpenID to a contact.
func GoldFeishu(dst *data.Store) (GoldStats, error) {
	var st GoldStats
	since, err := dst.GovernCursor(data.StageGold, sources.VendorFeishu, kindFeishuMessage)
	if err != nil {
		return st, err
	}
	msgs, maxUpdated, err := dst.SilverFeishuMessagesSince(since)
	if err != nil {
		return st, err
	}
	if len(msgs) == 0 {
		return st, nil
	}
	names, err := dst.SilverFeishuUserNames()
	if err != nil {
		return st, err
	}
	chats, err := dst.SilverFeishuChatMetas()
	if err != nil {
		return st, err
	}

	// Resolve an OpenID to a contact once per run (cache): the same person recurs
	// across many messages, and the cache also makes st.Contacts count each new
	// degree-2 contact exactly once.
	cache := map[string]string{}
	resolve := func(openID, name, tenantKey string) (string, error) {
		if openID == "" {
			return "", nil
		}
		if id, ok := cache[openID]; ok {
			return id, nil
		}
		if name == "" { // sender messages carry no name; fall back to the 二级用户 table
			name = names[openID]
		}
		id, created, err := dst.ResolveContact(sources.VendorFeishu, openID, name, tenantKey)
		if err != nil {
			return "", err
		}
		cache[openID] = id
		if created {
			st.Contacts++
		}
		return id, nil
	}

	threads := map[string]*data.GoldThread{}
	goldMsgs := make([]data.GoldMessage, 0, len(msgs))
	for _, m := range msgs {
		senderID, err := resolve(m.SenderOpenID, "", m.SenderTenantKey)
		if err != nil {
			return st, err
		}
		parts := make([]data.GoldParticipant, 0, 1+len(m.Mentions))
		if senderID != "" {
			parts = append(parts, data.GoldParticipant{ContactID: senderID, Role: "from"})
		}
		for _, mn := range m.Mentions {
			cid, err := resolve(mn.OpenID, mn.Name, mn.TenantKey)
			if err != nil {
				return st, err
			}
			if cid != "" {
				parts = append(parts, data.GoldParticipant{ContactID: cid, Role: "mention"})
			}
		}

		threadID := ""
		if m.ChatID != "" {
			threadID = "feishu:" + m.ChatID
			th := threads[m.ChatID]
			if th == nil {
				meta := chats[m.ChatID]
				th = &data.GoldThread{
					ID: threadID, Source: sources.VendorFeishu, AccountID: m.AccountID,
					ExternalID: m.ChatID, Kind: threadKind(meta.Mode), Title: meta.Name,
				}
				threads[m.ChatID] = th
			}
			if m.CreateTime > th.LastMessageAt {
				th.LastMessageAt = m.CreateTime
			}
		}

		goldMsgs = append(goldMsgs, data.GoldMessage{
			ID: "feishu:" + m.ExternalID, ThreadID: threadID, Source: sources.VendorFeishu,
			AccountID: m.AccountID, ExternalID: m.ExternalID, SenderContactID: senderID,
			BodyText: m.BodyText, SentAt: m.CreateTime,
			Fingerprint:  data.Fingerprint(threadID, senderID, m.BodyText, strconv.FormatInt(m.CreateTime, 10)),
			Participants: parts,
		})
	}

	threadRows := make([]data.GoldThread, 0, len(threads))
	for _, t := range threads {
		threadRows = append(threadRows, *t)
	}
	if err := dst.UpsertThreads(threadRows); err != nil {
		return st, err
	}
	if err := dst.UpsertMessages(goldMsgs); err != nil {
		return st, err
	}
	if err := dst.SaveGovernCursor(data.StageGold, sources.VendorFeishu, kindFeishuMessage, maxUpdated); err != nil {
		return st, err
	}
	st.Threads = len(threadRows)
	st.Messages = len(goldMsgs)
	return st, nil
}

// threadKind maps a 飞书 chat_mode to a gold thread kind.
func threadKind(mode string) string {
	if mode == "p2p" {
		return "dm"
	}
	return "group"
}

// kindIcloudContact labels the StageGold cursor for the iCloud address-book seed.
const kindIcloudContact = "icloud_contact"

// GoldContactsIcloud seeds the degree-1 address-book hub: each iCloud contact past
// the cursor becomes/refreshes a degree-1 contact with an email/phone channel per
// address, folding in any degree-2 contact a message already created on the same
// email/phone. Runs first so later email/phone resolves land on a known person.
func GoldContactsIcloud(dst *data.Store) (GoldStats, error) {
	var st GoldStats
	since, err := dst.GovernCursor(data.StageGold, sources.SourceICloud, kindIcloudContact)
	if err != nil {
		return st, err
	}
	contacts, maxUpdated, err := dst.SilverIcloudContactsSince(since)
	if err != nil {
		return st, err
	}
	if len(contacts) == 0 {
		return st, nil
	}
	for _, c := range contacts {
		if c.Deleted {
			continue
		}
		created, _, err := dst.UpsertAddressBookContact(c)
		if err != nil {
			return st, err
		}
		if created {
			st.Contacts++
		}
	}
	if err := dst.SaveGovernCursor(data.StageGold, sources.SourceICloud, kindIcloudContact, maxUpdated); err != nil {
		return st, err
	}
	return st, nil
}

// emailResolver returns a per-run cache that resolves an email address to a
// contact via contact_channels(platform='email'), counting each newly created
// contact once in st.
func emailResolver(dst *data.Store, st *GoldStats) func(addr, name string) (string, error) {
	cache := map[string]string{}
	return func(addr, name string) (string, error) {
		norm := data.NormEmail(addr)
		if norm == "" {
			return "", nil
		}
		if id, ok := cache[norm]; ok {
			return id, nil
		}
		id, created, err := dst.ResolveContact("email", norm, name, "")
		if err != nil {
			return "", err
		}
		cache[norm] = id
		if created {
			st.Contacts++
		}
		return id, nil
	}
}

// GoldMicrosoftMail fuses silver MS mail into gold threads/messages/participants:
// sender + recipients resolve by email, and a conversation groups a mail thread.
func GoldMicrosoftMail(dst *data.Store) (GoldStats, error) {
	var st GoldStats
	since, err := dst.GovernCursor(data.StageGold, sources.VendorMicrosoft, kindMSMail)
	if err != nil {
		return st, err
	}
	mails, maxUpdated, err := dst.SilverMicrosoftMailSince(since)
	if err != nil {
		return st, err
	}
	if len(mails) == 0 {
		return st, nil
	}
	resolve := emailResolver(dst, &st)

	threads := map[string]*data.GoldThread{}
	goldMsgs := make([]data.GoldMessage, 0, len(mails))
	for _, m := range mails {
		senderID, err := resolve(m.FromAddr, m.FromName)
		if err != nil {
			return st, err
		}
		parts := make([]data.GoldParticipant, 0, 1+len(m.ToRecipients))
		if senderID != "" {
			parts = append(parts, data.GoldParticipant{ContactID: senderID, Role: "from"})
		}
		for _, to := range m.ToRecipients {
			cid, err := resolve(to.Addr, to.Name)
			if err != nil {
				return st, err
			}
			if cid != "" {
				parts = append(parts, data.GoldParticipant{ContactID: cid, Role: "to"})
			}
		}

		threadID := ""
		if m.ConversationID != "" {
			threadID = "microsoft:" + m.ConversationID
			th := threads[m.ConversationID]
			if th == nil {
				th = &data.GoldThread{
					ID: threadID, Source: sources.VendorMicrosoft, AccountID: m.AccountID,
					ExternalID: m.ConversationID, Kind: "mail_thread", Title: m.Subject,
				}
				threads[m.ConversationID] = th
			}
			if m.ReceivedAt > th.LastMessageAt {
				th.LastMessageAt = m.ReceivedAt
			}
		}

		goldMsgs = append(goldMsgs, data.GoldMessage{
			ID: "microsoft:" + m.ExternalID, ThreadID: threadID, Source: sources.VendorMicrosoft,
			AccountID: m.AccountID, ExternalID: m.ExternalID, MsgKind: "email", Subject: m.Subject,
			SenderContactID: senderID, BodyText: m.BodyPreview, SentAt: m.ReceivedAt,
			Fingerprint:  data.Fingerprint(threadID, senderID, m.Subject, strconv.FormatInt(m.ReceivedAt, 10)),
			Participants: parts,
		})
	}

	threadRows := make([]data.GoldThread, 0, len(threads))
	for _, t := range threads {
		threadRows = append(threadRows, *t)
	}
	if err := dst.UpsertThreads(threadRows); err != nil {
		return st, err
	}
	if err := dst.UpsertMessages(goldMsgs); err != nil {
		return st, err
	}
	if err := dst.SaveGovernCursor(data.StageGold, sources.VendorMicrosoft, kindMSMail, maxUpdated); err != nil {
		return st, err
	}
	st.Threads = len(threadRows)
	st.Messages = len(goldMsgs)
	return st, nil
}

// GoldAgentMail fuses silver AgentMail into gold messages, resolving sender +
// recipients by email. AgentMail silver carries no conversation id, so v1 leaves
// messages thread-less (thread_id=”); mail threading lands later.
func GoldAgentMail(dst *data.Store) (GoldStats, error) {
	var st GoldStats
	since, err := dst.GovernCursor(data.StageGold, sources.VendorAgentMail, kindAgentMail)
	if err != nil {
		return st, err
	}
	mails, maxUpdated, err := dst.SilverAgentMailSince(since)
	if err != nil {
		return st, err
	}
	if len(mails) == 0 {
		return st, nil
	}
	resolve := emailResolver(dst, &st)

	goldMsgs := make([]data.GoldMessage, 0, len(mails))
	for _, m := range mails {
		senderID, err := resolve(m.FromEmail, m.FromName)
		if err != nil {
			return st, err
		}
		parts := make([]data.GoldParticipant, 0, 1+len(m.ToRecipients))
		if senderID != "" {
			parts = append(parts, data.GoldParticipant{ContactID: senderID, Role: "from"})
		}
		for _, to := range m.ToRecipients {
			cid, err := resolve(to.Addr, to.Name)
			if err != nil {
				return st, err
			}
			if cid != "" {
				parts = append(parts, data.GoldParticipant{ContactID: cid, Role: "to"})
			}
		}
		goldMsgs = append(goldMsgs, data.GoldMessage{
			ID: "agentmail:" + m.ExternalID, ThreadID: "", Source: sources.VendorAgentMail,
			AccountID: m.AccountID, ExternalID: m.ExternalID, MsgKind: "email", Subject: m.Subject,
			SenderContactID: senderID, BodyText: m.Snippet, SentAt: m.CreatedAtSrc,
			Fingerprint:  data.Fingerprint("", senderID, m.Subject, strconv.FormatInt(m.CreatedAtSrc, 10)),
			Participants: parts,
		})
	}
	if err := dst.UpsertMessages(goldMsgs); err != nil {
		return st, err
	}
	if err := dst.SaveGovernCursor(data.StageGold, sources.VendorAgentMail, kindAgentMail, maxUpdated); err != nil {
		return st, err
	}
	st.Messages = len(goldMsgs)
	return st, nil
}

// GoldFeishuEvents fuses silver 飞书 calendar events into gold calendar_events,
// resolving the organizer by open_id. attendees are reserved (empty) in silver.
func GoldFeishuEvents(dst *data.Store) (GoldStats, error) {
	var st GoldStats
	since, err := dst.GovernCursor(data.StageGold, sources.VendorFeishu, kindFeishuCalendarEvent)
	if err != nil {
		return st, err
	}
	events, maxUpdated, err := dst.SilverFeishuEventsSince(since)
	if err != nil {
		return st, err
	}
	if len(events) == 0 {
		return st, nil
	}
	goldEvents := make([]data.GoldEvent, 0, len(events))
	for _, e := range events {
		organizerID := ""
		if e.OrganizerOpenID != "" {
			id, created, err := dst.ResolveContact(sources.VendorFeishu, e.OrganizerOpenID, e.OrganizerName, "")
			if err != nil {
				return st, err
			}
			organizerID = id
			if created {
				st.Contacts++
			}
		}
		goldEvents = append(goldEvents, data.GoldEvent{
			ID: "feishu:" + e.ExternalID, Source: sources.VendorFeishu, AccountID: e.AccountID,
			ExternalID: e.ExternalID, CalendarID: e.CalendarID, Title: e.Subject, Location: e.Location,
			StartsAt: e.StartsAt, EndsAt: e.EndsAt, AllDay: e.AllDay, RRule: e.Recurrence,
			OrganizerContactID: organizerID, Status: e.Status,
			Fingerprint: data.Fingerprint(e.Subject, strconv.FormatInt(e.StartsAt, 10), strconv.FormatInt(e.EndsAt, 10)),
		})
	}
	if err := dst.UpsertCalendarEvents(goldEvents); err != nil {
		return st, err
	}
	if err := dst.SaveGovernCursor(data.StageGold, sources.VendorFeishu, kindFeishuCalendarEvent, maxUpdated); err != nil {
		return st, err
	}
	st.Events = len(goldEvents)
	return st, nil
}

// GoldMicrosoftEvents fuses silver MS calendar events into gold calendar_events,
// resolving organizer + every attendee by email. The shared fingerprint
// (title+start+end) lets a 飞书 and an MS copy of one meeting group at read time.
func GoldMicrosoftEvents(dst *data.Store) (GoldStats, error) {
	var st GoldStats
	since, err := dst.GovernCursor(data.StageGold, sources.VendorMicrosoft, kindMSEvent)
	if err != nil {
		return st, err
	}
	events, maxUpdated, err := dst.SilverMicrosoftEventsSince(since)
	if err != nil {
		return st, err
	}
	if len(events) == 0 {
		return st, nil
	}
	resolve := emailResolver(dst, &st)

	goldEvents := make([]data.GoldEvent, 0, len(events))
	for _, e := range events {
		organizerID, err := resolve(e.OrganizerAddr, e.OrganizerName)
		if err != nil {
			return st, err
		}
		atts := make([]data.GoldEventAttendee, 0, len(e.Attendees))
		for _, a := range e.Attendees {
			cid, err := resolve(a.Addr, a.Name)
			if err != nil {
				return st, err
			}
			if cid != "" {
				atts = append(atts, data.GoldEventAttendee{ContactID: cid, Response: a.Response})
			}
		}
		goldEvents = append(goldEvents, data.GoldEvent{
			ID: "microsoft:" + e.ExternalID, Source: sources.VendorMicrosoft, AccountID: e.AccountID,
			ExternalID: e.ExternalID, CalendarID: e.CalendarID, Title: e.Subject, Location: e.Location,
			StartsAt: e.StartsAt, EndsAt: e.EndsAt, AllDay: e.AllDay, RRule: e.Recurrence,
			OrganizerContactID: organizerID, Status: "",
			Fingerprint: data.Fingerprint(e.Subject, strconv.FormatInt(e.StartsAt, 10), strconv.FormatInt(e.EndsAt, 10)),
			Attendees:   atts,
		})
	}
	if err := dst.UpsertCalendarEvents(goldEvents); err != nil {
		return st, err
	}
	if err := dst.SaveGovernCursor(data.StageGold, sources.VendorMicrosoft, kindMSEvent, maxUpdated); err != nil {
		return st, err
	}
	st.Events = len(goldEvents)
	return st, nil
}

// GoldMicrosoftTodos fuses silver MS to-dos into gold todos. To-dos are not
// people, so there is no identity resolution — just a faithful source→gold copy
// on the (source, account_id, external_id) grain, with a fingerprint ready for
// future cross-source dedup. Only Microsoft supplies to-dos today.
func GoldMicrosoftTodos(dst *data.Store) (GoldStats, error) {
	var st GoldStats
	since, err := dst.GovernCursor(data.StageGold, sources.VendorMicrosoft, kindMSTodo)
	if err != nil {
		return st, err
	}
	todos, maxUpdated, err := dst.SilverMicrosoftTodosSince(since)
	if err != nil {
		return st, err
	}
	if len(todos) == 0 {
		return st, nil
	}
	goldTodos := make([]data.GoldTodo, 0, len(todos))
	for _, td := range todos {
		goldTodos = append(goldTodos, data.GoldTodo{
			ID: "microsoft:" + td.ExternalID, Source: sources.VendorMicrosoft, AccountID: td.AccountID,
			ExternalID: td.ExternalID, ListID: td.ListID, Title: td.Title, Notes: td.Body,
			Status: td.Status, Priority: td.Importance, DueAt: td.DueAt, CompletedAt: td.CompletedAt,
			Fingerprint: data.Fingerprint(td.Title, strconv.FormatInt(td.DueAt, 10)),
		})
	}
	if err := dst.UpsertTodos(goldTodos); err != nil {
		return st, err
	}
	if err := dst.SaveGovernCursor(data.StageGold, sources.VendorMicrosoft, kindMSTodo, maxUpdated); err != nil {
		return st, err
	}
	st.Todos = len(goldTodos)
	return st, nil
}
