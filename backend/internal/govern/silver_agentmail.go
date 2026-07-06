package govern

import (
	"encoding/json"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// silver_agentmail.go — 腾讯 Agent Mail governor + parser (issue #399).

// Bronze kind identifier for 腾讯 Agent Mail.
const kindAgentMail = "agentmail_mail"

func SilverAgentMail(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorAgentMail, kindAgentMail, parseAgentMail, dst.UpsertAgentMail)
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
