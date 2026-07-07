package govern

import (
	"encoding/json"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// silver_microsoft.go — Microsoft Graph governors (mail / event / todo) + parsers
// + Graph payload helpers (issue #399).

// Bronze kind identifiers for Microsoft Graph.
const (
	kindMSMail  = "ms_mail"
	kindMSEvent = "ms_event"
	kindMSTodo  = "ms_todo"
)

func SilverMicrosoftMail(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorMicrosoft, kindMSMail, parseMicrosoftMail, dst.UpsertMicrosoftMail)
}
func SilverMicrosoftEvents(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorMicrosoft, kindMSEvent, parseMicrosoftEvent, dst.UpsertMicrosoftEvents)
}
func SilverMicrosoftTodos(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorMicrosoft, kindMSTodo, parseMicrosoftTodo, dst.UpsertMicrosoftTodos)
}

// ---- Graph payload fragments + helpers ----

type graphEmail struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type graphDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

func parseGraphDT(g *graphDateTime) int64 {
	if g == nil {
		return 0
	}
	return parseISOTime(g.DateTime)
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

// ---- parsers ----

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
		Attendees: att, Recurrence: rawJSON(e.Recurrence), RecurrenceStd: NormalizeGraphRecurrence(rawJSON(e.Recurrence)),
		Deleted: r.Deleted, UpdatedAt: r.FetchedAt,
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
		Recurrence: rawJSON(t.Recurrence), RecurrenceStd: NormalizeGraphRecurrence(rawJSON(t.Recurrence)),
		ChecklistItems:  rawJSONArr(t.ChecklistItems),
		LinkedResources: rawJSONArr(t.LinkedResources), Deleted: r.Deleted, UpdatedAt: r.FetchedAt,
	}}
}
