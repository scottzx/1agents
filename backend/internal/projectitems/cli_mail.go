package projectitems

// CLI surface for Workspace Inbox mail tools (#205 / #206):
//
//	1agents project-items mail list|check|get|send|deliver|accept|archive|read|unread|targets|import-agentmail
//
// Mirrors the MCP mail tools via the same Client, so codex/bash agents that
// cannot call MCP still get full check → send → accept / agentmail intake.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func init() {
	cliVerbs["mail"] = true
}

const mailUsage = `mail subcommands (Workspace Inbox; cwd/ONEAGENTS_WORKSPACE_ID locks the box):

  mail list|check   [--status unread|read|all] [--archived] [--json]
  mail get <id>     [--json]
  mail send         --to <workspaceId> --title T [--content C] [--summary S] [--url U] [--from-ref R]
  mail deliver      --title T [--to workspaceId] [--source email|function|data_source|…]
                    [--content C] [--summary S] [--url U] [--from-ref R]   # function/data_source path
  mail accept <id>  [--title T] [--description D] [--priority P]
  mail archive <id> [--reason R]
  mail read <id>
  mail unread <id>
  mail targets      [--json]
  mail import-agentmail [--to default] [--limit N] [--json]
                    # shell out to agently-cli message +list → deliver each as source=email unread
`

func cliMail(args []string) int {
	if len(args) == 0 {
		fmt.Print(mailUsage)
		return 1
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Print(mailUsage)
		return 0
	case "list", "check":
		return cliMailList(args[1:])
	case "get":
		return cliMailGet(args[1:])
	case "send":
		return cliMailSend(args[1:])
	case "deliver":
		return cliMailDeliver(args[1:])
	case "accept":
		return cliMailAccept(args[1:])
	case "archive":
		return cliMailArchive(args[1:])
	case "read", "unread":
		return cliMailStatus(args[0], args[1:])
	case "targets":
		return cliMailTargets(args[1:])
	case "import-agentmail":
		return cliMailImportAgentMail(args[1:])
	default:
		return cliFail("unknown mail verb %q\n%s", args[0], mailUsage)
	}
}

func cliMailList(args []string) int {
	fs := flag.NewFlagSet("mail list", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	status := fs.String("status", "", "unread|read|all")
	archived := fs.Bool("archived", false, "include archived")
	asJSON := fs.Bool("json", false, "machine-readable")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c, wsID, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	st, body, err := c.ListInbox(*archived)
	if err != nil {
		return cliFail("mail list: %v", err)
	}
	if st != 200 {
		return cliFail("mail list failed (%d): %s", st, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Items  []map[string]any `json:"items"`
		Unread int              `json:"unread"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return emitRaw(st, body, nil)
	}
	want := strings.ToLower(strings.TrimSpace(*status))
	items := payload.Items
	if want != "" && want != "all" {
		filtered := make([]map[string]any, 0, len(items))
		for _, it := range items {
			if strings.EqualFold(fmt.Sprint(it["status"]), want) {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	if *asJSON {
		return printCLIJSON(map[string]any{
			"workspaceId": wsID, "count": len(items), "unread": payload.Unread, "items": items,
		})
	}
	fmt.Printf("workspace=%s unread=%d count=%d\n", wsID, payload.Unread, len(items))
	if len(items) == 0 {
		fmt.Println("(empty inbox)")
		return 0
	}
	for _, it := range items {
		id, _ := it["id"].(string)
		src, _ := it["source"].(string)
		stt, _ := it["status"].(string)
		title, _ := it["title"].(string)
		if title == "" {
			title, _ = it["summary"].(string)
		}
		if title == "" {
			title, _ = it["content"].(string)
		}
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		fmt.Printf("%-28s %-12s %-8s %s\n", id, src, stt, title)
	}
	return 0
}

func cliMailGet(args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("mail get", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	_ = fs.Bool("json", false, "machine-readable")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("mail get requires <id>")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.GetInboxItem(id))
}

func cliMailSend(args []string) int {
	fs := flag.NewFlagSet("mail send", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	to := fs.String("to", "", "recipient workspace id")
	title := fs.String("title", "", "title")
	content := fs.String("content", "", "body")
	summary := fs.String("summary", "", "summary")
	url := fs.String("url", "", "url")
	fromRef := fs.String("from-ref", "", "optional producer label")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*to) == "" || strings.TrimSpace(*title) == "" {
		return cliFail("mail send requires --to and --title")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	st, body, err := c.SendMail(SendMailArgs{
		ToWorkspaceID: *to,
		Title:         *title,
		Content:       *content,
		Summary:       *summary,
		URL:           *url,
		FromRef:       *fromRef,
	})
	return emitRaw(st, body, err)
}

func cliMailDeliver(args []string) int {
	fs := flag.NewFlagSet("mail deliver", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default box when --to empty)")
	to := fs.String("to", "", "recipient workspace id (default: current)")
	source := fs.String("source", "function", "source: function|data_source|email|agent|…")
	title := fs.String("title", "", "title")
	content := fs.String("content", "", "body")
	summary := fs.String("summary", "", "summary")
	urlStr := fs.String("url", "", "url")
	fromRef := fs.String("from-ref", "", "producer id (e.g. agentmail:msg_xxx)")
	fromWS := fs.String("from-workspace", "", "optional sender workspace")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*title) == "" && strings.TrimSpace(*content) == "" && strings.TrimSpace(*urlStr) == "" {
		return cliFail("mail deliver requires --title, --content or --url")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.DeliverEnvelope(DeliverEnvelopeArgs{
		ToWorkspaceID:   *to,
		Source:          *source,
		FromWorkspaceID: *fromWS,
		FromRef:         *fromRef,
		Title:           *title,
		Content:         *content,
		URL:             *urlStr,
		Summary:         *summary,
	}))
}

func cliMailAccept(args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("mail accept", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	title := fs.String("title", "", "override title")
	desc := fs.String("description", "", "override description")
	priority := fs.String("priority", "", "urgent|high|medium|low")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("mail accept requires <id>")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.AcceptMail(id, *title, *desc, *priority))
}

func cliMailArchive(args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("mail archive", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	reason := fs.String("reason", "", "optional reason (printed only)")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("mail archive requires <id>")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	st, body, err := c.ArchiveMail(id)
	if err != nil || st != 200 {
		return emitRaw(st, body, err)
	}
	if strings.TrimSpace(*reason) != "" {
		fmt.Printf("archived %s reason=%s\n", id, *reason)
		return 0
	}
	return emitRaw(st, body, nil)
}

func cliMailStatus(action string, args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("mail "+action, flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("mail %s requires <id>", action)
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.SetMailStatus(id, action))
}

func cliMailTargets(args []string) int {
	fs := flag.NewFlagSet("mail targets", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	asJSON := fs.Bool("json", false, "machine-readable")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	st, body, err := c.ListMailTargets()
	if err != nil || st != 200 {
		return emitRaw(st, body, err)
	}
	if *asJSON {
		return emitRaw(st, body, nil)
	}
	var payload struct {
		Targets []struct {
			ProjectID string `json:"projectId"`
			Name      string `json:"name"`
		} `json:"targets"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return emitRaw(st, body, nil)
	}
	if len(payload.Targets) == 0 {
		fmt.Println("(no targets)")
		return 0
	}
	for _, t := range payload.Targets {
		fmt.Printf("%-36s %s\n", t.ProjectID, t.Name)
	}
	return 0
}

// cliMailImportAgentMail shells out to agently-cli and delivers each message
// into the target Workspace Inbox as source=email, status=unread (default),
// from_ref=agentmail:<message_id>. Dedup is best-effort via from_ref uniqueness
// on re-import (daemon still inserts; callers may archive duplicates).
func cliMailImportAgentMail(args []string) int {
	fs := flag.NewFlagSet("mail import-agentmail", flag.ContinueOnError)
	project := fs.String("project", "", "unused; prefer --to")
	to := fs.String("to", "default", "recipient workspace id (default assistant box)")
	limit := fs.Int("limit", 20, "max messages from agently-cli")
	asJSON := fs.Bool("json", false, "machine-readable summary")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	_ = project
	out, err := exec.Command("agently-cli", "message", "+list", "--dir", "inbox",
		"--limit", fmt.Sprint(*limit)).Output()
	if err != nil {
		return cliFail("agently-cli message +list: %v\n(ensure agently-cli is on PATH and authorized)", err)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Data []json.RawMessage `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil || !env.OK {
		return cliFail("parse agently-cli output: %v (ok=%v)", err, env.OK)
	}
	// Resolve client against --to workspace so DeliverEnvelope default box is correct.
	baseURL, err := resolveBaseURL()
	if err != nil {
		return cliFail("%v", err)
	}
	wsID := strings.TrimSpace(*to)
	if wsID == "" {
		wsID = "default"
	}
	c := NewClient(baseURL, wsID, "")

	type result struct {
		MessageID string `json:"messageId"`
		InboxID   string `json:"inboxId,omitempty"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		Error     string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(env.Data.Data))
	delivered := 0
	for _, raw := range env.Data.Data {
		var m struct {
			MessageID string `json:"message_id"`
			Subject   string `json:"subject"`
			Snippet   string `json:"snippet"`
			CreatedAt string `json:"created_at"`
			IsRead    bool   `json:"is_read"`
			From      struct {
				Email string `json:"email"`
				Name  string `json:"name"`
			} `json:"from"`
		}
		if json.Unmarshal(raw, &m) != nil || m.MessageID == "" {
			continue
		}
		title := strings.TrimSpace(m.Subject)
		if title == "" {
			title = "Agent Mail " + m.MessageID
		}
		fromLabel := m.From.Name
		if fromLabel == "" {
			fromLabel = m.From.Email
		}
		content := m.Snippet
		if fromLabel != "" {
			content = "From: " + fromLabel + " <" + m.From.Email + ">\n\n" + m.Snippet
		}
		// Always deliver as unread into the Workspace Inbox (product unread),
		// independent of Agent Mail's own is_read flag — intake means "to triage".
		st, body, err := c.DeliverEnvelope(DeliverEnvelopeArgs{
			ToWorkspaceID: wsID,
			Source:        "email",
			FromRef:       "agentmail:" + m.MessageID,
			Title:         title,
			Content:       content,
			Summary:       m.Snippet,
			Tags:          []string{"agentmail", "data_source"},
		})
		r := result{MessageID: m.MessageID, Title: title, Status: "failed"}
		if err != nil {
			r.Error = err.Error()
			results = append(results, r)
			continue
		}
		if st != 200 {
			r.Error = fmt.Sprintf("HTTP %d: %s", st, strings.TrimSpace(string(body)))
			results = append(results, r)
			continue
		}
		var item struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		_ = json.Unmarshal(body, &item)
		r.InboxID = item.ID
		r.Status = item.Status
		if r.Status == "" {
			r.Status = "unread"
		}
		delivered++
		results = append(results, r)
		_ = m.CreatedAt
	}
	if *asJSON {
		return printCLIJSON(map[string]any{
			"workspaceId": wsID,
			"fetched":     len(env.Data.Data),
			"delivered":   delivered,
			"at":          time.Now().UTC().Format(time.RFC3339),
			"items":       results,
		})
	}
	fmt.Printf("import-agentmail → workspace=%s delivered=%d/%d\n", wsID, delivered, len(env.Data.Data))
	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(os.Stderr, "  FAIL %s: %s\n", r.MessageID, r.Error)
			continue
		}
		fmt.Printf("  %s  %s  status=%s  %s\n", r.InboxID, r.MessageID, r.Status, r.Title)
	}
	if delivered == 0 && len(env.Data.Data) > 0 {
		return 1
	}
	return 0
}
