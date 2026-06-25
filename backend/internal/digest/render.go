package digest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// cst is UTC+8, the timezone chat timestamps are rendered in.
var cst = time.FixedZone("UTC+8", 8*3600)

// messageText extracts a flat, single-line-ish human string from a message's
// raw body content per its type. Best-effort: unknown/unparseable types degrade
// to a "[msg_type]" placeholder rather than dumping raw JSON.
func messageText(m feishu.Message) string {
	switch m.MsgType {
	case "text":
		var t struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal([]byte(m.Content), &t)
		return t.Text
	case "post":
		var p struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag  string `json:"tag"`
				Text string `json:"text"`
				Href string `json:"href"`
			} `json:"content"`
		}
		_ = json.Unmarshal([]byte(m.Content), &p)
		parts := []string{}
		if p.Title != "" {
			parts = append(parts, p.Title)
		}
		for _, blk := range p.Content {
			for _, el := range blk {
				if el.Text != "" {
					parts = append(parts, el.Text)
				}
				if el.Href != "" {
					parts = append(parts, el.Href)
				}
			}
		}
		return strings.Join(parts, " ")
	case "image":
		return "[图片]"
	case "system":
		return "[系统消息]"
	case "video_chat":
		return "[视频通话]"
	default:
		return "[" + m.MsgType + "]"
	}
}

// senderLabel prefers the resolved name; falls back to a short open_id suffix.
func senderLabel(m feishu.Message) string {
	if m.SenderName != "" {
		return m.SenderName
	}
	id := m.SenderID
	if len(id) > 6 {
		return "…" + id[len(id)-6:]
	}
	return id
}

// RenderBatch formats messages as one line each: "MM-DD HH:MM 发言人（type）: text"
// in chronological order, newlines in content collapsed to spaces.
func RenderBatch(msgs []feishu.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		ts := time.UnixMilli(m.CreateTime).In(cst).Format("01-02 15:04")
		txt := strings.ReplaceAll(messageText(m), "\n", " ")
		fmt.Fprintf(&b, "%s %s（%s）: %s\n", ts, senderLabel(m), m.MsgType, txt)
	}
	return b.String()
}

// BuildAnalysisPrompt assembles the agent instruction: the resolved value
// standard(s) followed by the rendered chat batch. This text is what the
// analysis task injects (same path as buildIssueBackground).
func BuildAnalysisPrompt(chatName string, templates []meta.DigestTemplate, msgs []feishu.Message) string {
	var b strings.Builder
	b.WriteString("# 任务：从下面的群聊记录中提取有价值的内容\n\n")
	b.WriteString("## 价值标准（请严格按以下模板定义的「什么算有价值」和输出结构来提取）\n\n")
	if len(templates) == 0 {
		b.WriteString("（该群未配置模板，按通用社群常识提取：项目自荐、招募找人、资源链接、运营公告、待办。）\n\n")
	}
	for _, t := range templates {
		b.WriteString("### 模板：" + t.Name + "\n\n")
		b.WriteString(strings.TrimSpace(t.BodyMD))
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "## 群「%s」聊天记录（共 %d 条，时间升序，UTC+8）\n\n", chatName, len(msgs))
	b.WriteString(RenderBatch(msgs))
	return b.String()
}
