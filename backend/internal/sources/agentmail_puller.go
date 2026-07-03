package sources

// agentmail_puller.go pulls the 腾讯 Agent Mail inbox into the bronze layer via
// the shared Store.Sync driver. Authorization is external: agently-cli holds the
// mailbox token (keychain), so the puller only shells out to
// `agently-cli message +list` and stores each returned message verbatim as
// application/json — governance shapes it later, same contract as every other
// puller.
//
// Change detection is a created_at watermark: the cursor is the newest mail's
// RFC3339 timestamp, passed back as --after on the next sync so only newer mail
// is fetched. The boundary is inclusive; bronze's per-record ETag (a content
// hash) drops the unchanged overlap row, so no mail is missed or duplicated.
//
// Inbound only — sending/replying is not part of the fetch layer.

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const (
	agentMailBin   = "agently-cli"
	agentMailDir   = "inbox"
	agentMailLimit = 50 // CLI page cap
	agentMailKind  = "agentmail_mail"
	agentMailColl  = "inbox"
)

// pullTimeoutAgentMail bounds a single `agently-cli message +list` invocation.
const pullTimeoutAgentMail = 60 * time.Second

// agentmailPuller pulls one account's Agent Mail inbox. runList is the injectable
// exec seam (production shells out to agently-cli; tests feed a canned envelope),
// so the whole Discover→Pull→bronze path is offline-testable.
type agentmailPuller struct {
	runList func(ctx context.Context, after string) ([]byte, error)
}

// NewAgentMailPuller builds a puller that shells out to agently-cli. The account
// is a registry marker only — agently-cli owns the credential — so accountID and
// region are unused here (kept in the driver signature for parity with the other
// vendors).
func NewAgentMailPuller() Puller {
	p := &agentmailPuller{}
	p.runList = p.execList
	return p
}

func (p *agentmailPuller) Source() string { return VendorAgentMail }

// Discover exposes the single inbox collection. Gate stays "" — Agent Mail has no
// collection version, so the driver always pulls (incremental is the watermark).
func (p *agentmailPuller) Discover(accountID string) ([]Collection, error) {
	return []Collection{{Kind: agentMailKind, ID: agentMailColl}}, nil
}

// Pull fetches inbox mail newer than the cursor watermark, one page, verbatim.
func (p *agentmailPuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pullTimeoutAgentMail)
	defer cancel()

	out, err := p.runList(ctx, cur.Value)
	if err != nil {
		return nil, cur, true, err
	}
	msgs, err := parseAgentMailList(out)
	if err != nil {
		return nil, cur, true, err
	}

	recs := make([]RawRecord, 0, len(msgs))
	watermark := cur.Value
	for _, m := range msgs {
		if m.ID == "" {
			continue // no stable upstream id → skip rather than fabricate one
		}
		recs = append(recs, RawRecord{
			Kind:        agentMailKind,
			Collection:  agentMailColl,
			UID:         m.ID,
			ETag:        hashHex(m.Raw), // content hash → CommitPage skips unchanged rows
			ContentType: "application/json",
			Payload:     string(m.Raw),
		})
		if m.CreatedAt != "" && m.CreatedAt > watermark {
			watermark = m.CreatedAt // RFC3339 sorts lexically == chronologically
		}
	}
	// Single page (the CLI returns the whole inbox window at once): done=true.
	return recs, Cursor{Kind: "timestamp", Value: watermark}, true, nil
}

// execList runs `agently-cli message +list --dir inbox --limit N [--after ts]`.
func (p *agentmailPuller) execList(ctx context.Context, after string) ([]byte, error) {
	args := []string{"message", "+list", "--dir", agentMailDir, "--limit", fmt.Sprint(agentMailLimit)}
	if after != "" {
		args = append(args, "--after", after)
	}
	out, err := exec.CommandContext(ctx, agentMailBin, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("agentmail: %s message +list: %w", agentMailBin, err)
	}
	return out, nil
}

// agentMailMsg is one parsed message: the fields the puller needs (id, created_at)
// plus the verbatim JSON bytes stored to bronze.
type agentMailMsg struct {
	ID        string
	CreatedAt string
	Raw       json.RawMessage
}

// agentMailEnvelope mirrors `agently-cli message +list` stdout: {ok, data:{data:[…]}}.
type agentMailEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Data []json.RawMessage `json:"data"`
	} `json:"data"`
}

// parseAgentMailList decodes the CLI envelope into messages, keeping each message
// object verbatim for bronze. A missing message_id leaves ID empty (Pull skips it).
func parseAgentMailList(raw []byte) ([]agentMailMsg, error) {
	var env agentMailEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("agentmail: parse +list output: %w", err)
	}
	if !env.OK {
		return nil, fmt.Errorf("agentmail: agently-cli returned ok=false")
	}
	msgs := make([]agentMailMsg, 0, len(env.Data.Data))
	for _, rawMsg := range env.Data.Data {
		var fields struct {
			MessageID string `json:"message_id"`
			CreatedAt string `json:"created_at"`
		}
		_ = json.Unmarshal(rawMsg, &fields) // best-effort; verbatim bytes are the source of truth
		msgs = append(msgs, agentMailMsg{
			ID:        fields.MessageID,
			CreatedAt: fields.CreatedAt,
			Raw:       rawMsg,
		})
	}
	return msgs, nil
}
