package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// CLIRunner runs the Feishu CLI with the given args and returns stdout. It is a
// seam so tests can inject canned responses instead of shelling out.
type CLIRunner func(ctx context.Context, args ...string) ([]byte, error)

// defaultCLIRunner shells out to the `lark-cli` binary. lark-cli writes
// progress lines (e.g. "[page 1] fetching...") to stderr, so only stdout is
// returned; stderr is folded into the error on failure.
func defaultCLIRunner(bin string) CLIRunner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%s %v: %w: %s", bin, args, err, errb.String())
		}
		return out.Bytes(), nil
	}
}

// Client wraps lark-cli for one Feishu account (user identity).
type Client struct {
	run     CLIRunner
	account string // channel_acc_id stamped onto every fetched message
}

// NewClient builds a Client that shells out to bin (default "lark-cli") under
// account (default "default").
func NewClient(bin, account string) *Client {
	if bin == "" {
		bin = "lark-cli"
	}
	if account == "" {
		account = "default"
	}
	return &Client{run: defaultCLIRunner(bin), account: account}
}

// newClientWithRunner is used by tests to inject a fake CLIRunner.
func newClientWithRunner(run CLIRunner, account string) *Client {
	if account == "" {
		account = "default"
	}
	return &Client{run: run, account: account}
}

// ── lark-cli response shapes (only the fields we consume) ──

type apiMsgResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Items []struct {
			MessageID  string `json:"message_id"`
			MsgType    string `json:"msg_type"`
			CreateTime string `json:"create_time"` // epoch ms, as string
			Sender     struct {
				ID string `json:"id"`
			} `json:"sender"`
			Body struct {
				Content string `json:"content"`
			} `json:"body"`
		} `json:"items"`
	} `json:"data"`
}

type apiMembersResp struct {
	Code int `json:"code"`
	Data struct {
		Items []struct {
			MemberID string `json:"member_id"`
			Name     string `json:"name"`
		} `json:"items"`
	} `json:"data"`
}

type apiChatsResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Items []struct {
			ChatID      string `json:"chat_id"`
			Name        string `json:"name"`
			Avatar      string `json:"avatar"`
			Description string `json:"description"`
			External    bool   `json:"external"`
			ChatMode    string `json:"chat_mode"`
			TenantKey   string `json:"tenant_key"`
		} `json:"items"`
	} `json:"data"`
}

// ChatInfo is one Feishu group the logged-in user is a member of, as returned
// by lark-cli `im/v1/chats`. Name is often empty (Feishu omits it for many
// groups), so callers must fall back to Description then ChatID for display.
type ChatInfo struct {
	ChatID      string `json:"chatId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
	ChatMode    string `json:"chatMode"`
	TenantKey   string `json:"tenantKey"`
	External    bool   `json:"external"`
}

// DoctorCheck is one lark-cli health-check line (auth/connectivity). Status is
// e.g. "pass"/"warn"/"fail".
type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// ListChats returns every Feishu group the logged-in user is in. --page-all
// loops all pages internally, mirroring FetchMessages.
func (c *Client) ListChats(ctx context.Context) ([]ChatInfo, error) {
	out, err := c.run(ctx, "api", "GET", "/open-apis/im/v1/chats",
		"--params", `{"page_size":"50"}`, "--as", "user",
		"--page-all", "--page-limit", "0", "--format", "json")
	if err != nil {
		return nil, err
	}
	var resp apiChatsResp
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("feishu: decode chats: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: chats api code=%d msg=%q", resp.Code, resp.Msg)
	}
	chats := make([]ChatInfo, 0, len(resp.Data.Items))
	for _, it := range resp.Data.Items {
		chats = append(chats, ChatInfo{
			ChatID:      it.ChatID,
			Name:        it.Name,
			Avatar:      it.Avatar,
			Description: it.Description,
			ChatMode:    it.ChatMode,
			TenantKey:   it.TenantKey,
			External:    it.External,
		})
	}
	return chats, nil
}

// Doctor runs lark-cli's health check and returns the per-check results.
// Best-effort: on a hard failure the error is returned so the handler can
// surface a "未连接" state.
func (c *Client) Doctor(ctx context.Context) ([]DoctorCheck, error) {
	out, err := c.run(ctx, "doctor", "--format", "json")
	if err != nil {
		// doctor may not support --format on older builds; retry plain.
		out, err = c.run(ctx, "doctor")
		if err != nil {
			return nil, err
		}
	}
	var resp struct {
		Checks []DoctorCheck `json:"checks"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("feishu: decode doctor: %w", err)
	}
	return resp.Checks, nil
}

// FetchMessages pulls a chat's history via lark-cli, ascending by create time.
// startSec (epoch seconds) bounds the lower edge; <= 0 fetches from the start.
// --page-all aggregates every page's items into one response.
func (c *Client) FetchMessages(ctx context.Context, chatID string, startSec int64) ([]Message, error) {
	params := map[string]string{
		"container_id_type": "chat",
		"container_id":      chatID,
		"sort_type":         "ByCreateTimeAsc",
		"page_size":         "50",
	}
	if startSec > 0 {
		params["start_time"] = strconv.FormatInt(startSec, 10)
	}
	paramsJSON, _ := json.Marshal(params)
	out, err := c.run(ctx, "api", "GET", "/open-apis/im/v1/messages",
		"--params", string(paramsJSON), "--as", "user",
		"--page-all", "--page-limit", "0", "--format", "json")
	if err != nil {
		return nil, err
	}
	var resp apiMsgResp
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("feishu: decode messages: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: messages api code=%d msg=%q", resp.Code, resp.Msg)
	}
	msgs := make([]Message, 0, len(resp.Data.Items))
	for _, it := range resp.Data.Items {
		ct, _ := strconv.ParseInt(it.CreateTime, 10, 64)
		msgs = append(msgs, Message{
			Channel:      Channel,
			ChannelAccID: c.account,
			MessageID:    it.MessageID,
			SessionID:    chatID,
			SenderID:     it.Sender.ID,
			MsgType:      it.MsgType,
			Title:        extractTitle(it.MsgType, it.Body.Content),
			Content:      it.Body.Content,
			CreateTime:   ct,
		})
	}
	return msgs, nil
}

// FetchMembers returns an open_id → display name map for a chat. Works for
// external members too (unlike the contact API), since they are in the chat.
func (c *Client) FetchMembers(ctx context.Context, chatID string) (map[string]string, error) {
	out, err := c.run(ctx, "api", "GET", "/open-apis/im/v1/chats/"+chatID+"/members",
		"--params", `{"page_size":"100","member_id_type":"open_id"}`, "--as", "user",
		"--page-all", "--page-limit", "0", "--format", "json")
	if err != nil {
		return nil, err
	}
	var resp apiMembersResp
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("feishu: decode members: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: members api code=%d", resp.Code)
	}
	names := make(map[string]string, len(resp.Data.Items))
	for _, it := range resp.Data.Items {
		if it.Name != "" {
			names[it.MemberID] = it.Name
		}
	}
	return names, nil
}

// extractTitle pulls a human title out of a message body where one exists
// (currently 'post' messages carry a top-level "title"). Best-effort: returns
// "" on any parse miss.
func extractTitle(msgType, content string) string {
	if msgType != "post" || content == "" {
		return ""
	}
	var post struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(content), &post); err != nil {
		return ""
	}
	return post.Title
}
