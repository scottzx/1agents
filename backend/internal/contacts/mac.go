// mac.go adds the iMessage data channel: a sibling syncer of the Feishu one that
// reads the local chat.db (plaintext, gated by the user's Full Disk Access grant
// — no decryption) into unified_messages (channel='imessage'), resolving sender
// names from the address book by phone. iMessage has no official iCloud/remote
// API, so it stays local-only; contacts come via iCloud CardDAV (see icloud.go).
package contacts

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/imessage"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// imSession is the single watermark key for the iMessage pull: chat.db is local
// and quota-free, so one global high-water mark over message.date is simpler than
// the per-chat watermarks the Feishu syncer needs.
const imSession = "__all__"

// imBatchLimit caps one iMessage pull; repeated syncs walk forward via the
// watermark. Generous because the read is a single local query.
const imBatchLimit = 20000

// textContent wraps plain text in the same {"text": …} shape the Feishu text
// payload uses, so the shared preview/extraction logic treats every channel's
// text uniformly.
func textContent(s string) string {
	b, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{s})
	return string(b)
}

// SyncIMessage performs one incremental pull from chat.db into unified_messages
// (channel='imessage'): it reads messages past the stored watermark, resolves
// sender names from the address book by phone, dedup-inserts them, records each
// participant as an auto-linked iMessage channel, and advances the watermark.
func (h *Handler) SyncIMessage() (fetched, inserted int, watermark int64, err error) {
	if err := h.requireConsent(ModAppleIMessage); err != nil {
		return 0, 0, 0, err
	}
	rules := h.moduleRules(ModAppleIMessage)
	includeAttachments := ruleBool(rules, "includeAttachments", true)

	path := imessage.DefaultPath()
	wm, _, err := h.fs.GetWatermark(imessage.Channel, "default", imSession)
	if err != nil {
		return 0, 0, 0, err
	}
	// "last N days" scope rule → watermark floor (absent/0 = full history). It only
	// ever raises the floor, so widening the window later won't backfill older history.
	since := wm
	if days, ok := ruleInt(rules, "timeWindowDays"); ok && days > 0 {
		if floor := imessage.UnixMsToAppleDate(time.Now().AddDate(0, 0, -days).UnixMilli()); floor > since {
			since = floor
		}
	}
	msgs, maxApple, err := imessage.Read(path, since, imBatchLimit)
	if err != nil {
		return 0, 0, 0, err
	}
	names, err := h.cs.PhoneNameMap()
	if err != nil {
		return 0, 0, 0, err
	}

	// One latest sighting per incoming handle, for channel upsert.
	type seen struct {
		name    string
		session string
		last    int64
	}
	handles := map[string]seen{}

	fm := make([]feishu.Message, 0, len(msgs))
	for _, m := range msgs {
		if !includeAttachments && m.HasAttach && m.Text == "" {
			continue // attachment-only message excluded by crawl rule
		}
		senderID, senderName := m.Handle, ""
		if m.IsFromMe {
			senderID, senderName = "me", "我"
		} else if m.Handle != "" {
			senderName = names[meta.NormalizePhone(m.Handle)]
		}
		msgType := "text"
		if m.HasAttach {
			msgType = "attachment"
		}
		fm = append(fm, feishu.Message{
			Channel:      imessage.Channel,
			ChannelAccID: "default",
			MessageID:    m.GUID,
			SessionID:    m.ChatGUID,
			SenderID:     senderID,
			SenderName:   senderName,
			MsgType:      msgType,
			Content:      textContent(m.Text),
			CreateTime:   m.CreateTime,
		})
		if !m.IsFromMe && m.Handle != "" {
			if prev, ok := handles[m.Handle]; !ok || m.CreateTime >= prev.last {
				handles[m.Handle] = seen{name: senderName, session: m.ChatGUID, last: m.CreateTime}
			}
		}
	}

	inserted, err = h.fs.UpsertMessages(fm)
	if err != nil {
		return 0, 0, 0, err
	}
	fetched = len(msgs)

	for handle, hv := range handles {
		if err = h.cs.UpsertIMessageChannel(handle, hv.name, hv.session, hv.last); err != nil {
			return fetched, inserted, 0, err
		}
	}

	watermark = wm
	if maxApple > wm {
		if err = h.fs.SetWatermark(imessage.Channel, "default", imSession, maxApple,
			time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fetched, inserted, 0, err
		}
		watermark = maxApple
	}
	return fetched, inserted, watermark, nil
}

// HandleIMessageSync: POST /api/contacts/imessage/sync → pull chat.db. A read
// failure is usually the missing Full Disk Access grant; reported as 403.
func (h *Handler) HandleIMessageSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fetched, inserted, watermark, err := h.SyncIMessage()
	if errors.Is(err, errNotConsented) {
		http.Error(w, err.Error(), http.StatusPreconditionRequired)
		return
	}
	if err != nil {
		// A read failure here is almost always the missing Full Disk Access grant.
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fetched": fetched, "inserted": inserted, "watermark": watermark,
	})
}
