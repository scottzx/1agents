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
	Contacts int // newly created degree-2 contacts
}

// Gold runs every source's silver→gold fusion (v1: 飞书 only).
func Gold(dst *data.Store) (GoldStats, error) {
	return GoldFeishu(dst)
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
