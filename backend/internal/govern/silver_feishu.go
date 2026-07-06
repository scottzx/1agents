package govern

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// silver_feishu.go — 飞书 governors (message / chat / 二级用户) + parsers (issue #399).

// Bronze kind identifiers for 飞书 (declared where the pullers live; the catalog
// uses the same strings).
const (
	kindFeishuMessage    = "feishu_message"
	kindFeishuChat       = "feishu_chat"
	kindFeishuChatMember = "feishu_chat_member"
)

func SilverFeishuMessages(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorFeishu, kindFeishuMessage, parseFeishuMessage, dst.UpsertFeishuMessages)
}

func SilverFeishuChats(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.VendorFeishu, kindFeishuChat, parseFeishuChat, dst.UpsertFeishuChats)
}

// SilverFeishuUsers rebuilds 飞书联系人 by MERGING two discovery sources into one
// table: (1) every 飞书 message's sender + @mentions (name comes free from
// mentions[].name), and (2) the group roster (feishu_chat_member) — the members
// list pulled per chat via lark-cli, which also brings in members who never
// spoke and carries authoritative names. It is a full recompute over both
// streams each run (not cursor-gated): cheap at chat scale and always consistent.
func SilverFeishuUsers(src *sources.Store, dst *data.Store) (int, error) {
	recs, maxFetched, err := src.RecordsSince(sources.VendorFeishu, kindFeishuMessage, 0)
	if err != nil {
		return 0, err
	}
	members, memMax, err := src.RecordsSince(sources.VendorFeishu, kindFeishuChatMember, 0)
	if err != nil {
		return 0, err
	}
	if memMax > maxFetched {
		maxFetched = memMax
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
	// Group roster: one bronze record per (chat, member); Collection is the chat
	// id. "roster" discovery brings in members who never spoke + authoritative
	// names, but has no per-message timestamp — pass t=0.
	for _, r := range members {
		mem := decodeFeishuMember(r.Payload)
		if mem == nil {
			continue
		}
		touch(mem.MemberID, mem.Name, mem.TenantKey, "roster", r.Collection, 0)
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

func parseEpochMS(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// ---- 飞书 payload shapes + parsers ----

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

// feishuMember is one item from /im/v1/chats/:id/members (bronze feishu_chat_member).
type feishuMember struct {
	MemberID  string `json:"member_id"` // open_id (member_id_type=open_id)
	Name      string `json:"name"`
	TenantKey string `json:"tenant_key"`
}

func decodeFeishuMember(payload string) *feishuMember {
	var m feishuMember
	if json.Unmarshal([]byte(payload), &m) != nil || m.MemberID == "" {
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
