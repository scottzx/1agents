package projectitems

// This file implements the Bash-invokable CLI form of the project-items tools:
//
//	1agents project-items list|get|graph|create|update|close|reopen|discussion|milestones ...
//
// It exists because some agent engines (notably codex 0.142 in ACP app-server
// mode) cannot execute MCP tools but CAN run shell commands. The CLI hits the
// SAME daemon HTTP endpoints as the MCP server (via the shared Client), so it
// inherits identical business logic. Isolation here is project + user only
// (no session/task scope): the workspace is resolved from ONEAGENTS_WORKSPACE_ID,
// then --project, then the current directory; user isolation is inherent (the
// per-user daemon + loopback-only, no-auth-needed access).

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// cliVerbs is the set of first-arg tokens that route to the CLI. A bare
// `project-items` (no verb, what the MCP bridge spawns) is NOT here, so it falls
// through to the stdio MCP server — see main.go dispatch.
var cliVerbs = map[string]bool{
	"list": true, "get": true, "graph": true, "create": true, "update": true,
	"close": true, "reopen": true, "discussion": true, "milestones": true,
	"pdf":  true,
	"help": true, "-h": true, "--help": true,
}

// IsCLIVerb reports whether v selects the CLI (vs. the bare stdio-MCP invocation).
func IsCLIVerb(v string) bool { return cliVerbs[v] }

const cliUsage = `usage: 1agents project-items <verb> [flags]   (run inside a project directory)

  list                 [--status S] [--type T] [--json]
  get <id>             [--json]
  graph <id>           [--json]
  create               --title T [--type task|requirement|bug] [--priority P] [--milestone M]
                       [--assignee A] [--acceptance C] [--description D] [--json '<payload>']
  discussion           --title T [--description D]
  update <id>          [--status completed|cancelled] [--issue-state open|closed] [--priority P]
                       [--milestone M] [--type T] [--assignee A] [--acceptance C]
                       [--description D] [--json '<patch>']
  close <id>           (issueState=closed — how a requirement/bug is finished)
  reopen <id>          (issueState=open)
  milestones list
  milestones create    --name N [--description D] [--target-date RFC3339] [--predecessor ID]
  milestones update <id> [--name N] [--description D] [--target-date RFC3339] [--predecessor ID]
  pdf                  [--out PATH] [--font TTF] (导出当前项目看板为 PDF 报告)

common: --project <id|name|path> overrides cwd-based project resolution; --json prints machine output.
status values: completed|cancelled (runnable states are scheduler-owned). type: task|requirement|bug|discussion.`

// RunCLI dispatches a project-items CLI verb. args[0] is the verb.
func RunCLI(args []string) int {
	if len(args) == 0 {
		fmt.Println(cliUsage)
		return 1
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Println(cliUsage)
		return 0
	case "list":
		return cliList(args[1:])
	case "get":
		return cliGetLike(args[1:], false)
	case "graph":
		return cliGetLike(args[1:], true)
	case "create":
		return cliCreate(args[1:])
	case "discussion":
		return cliDiscussion(args[1:])
	case "update":
		return cliUpdate(args[1:])
	case "close":
		return cliCloseReopen(args[1:], "closed")
	case "reopen":
		return cliCloseReopen(args[1:], "open")
	case "milestones":
		return cliMilestones(args[1:])
	case "pdf":
		return cliPDF(args[1:])
	default:
		return cliFail("unknown verb %q\n%s", args[0], cliUsage)
	}
}

func cliFail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	return 1
}

// cliClient resolves base URL + workspace (from --project or cwd) and returns a
// ready Client. The returned wsID is the resolved workspace id.
func cliClient(project string) (*Client, string, int) {
	baseURL, err := resolveBaseURL()
	if err != nil {
		return nil, "", cliFail("%v", err)
	}
	wsID, err := resolveWorkspaceID(baseURL, project)
	if err != nil {
		return nil, "", cliFail("%v", err)
	}
	// No token: loopback bypasses auth (unless a Cloudflare tunnel is active, in
	// which case the daemon requires the in-memory local token, which a separate
	// process can't obtain — the CLI then gets 401).
	return NewClient(baseURL, wsID, ""), wsID, -1
}

// ── verbs ────────────────────────────────────────────────────────────────────

func cliList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default: cwd)")
	status := fs.String("status", "", "filter by status")
	typ := fs.String("type", "", "filter by type (task|requirement|bug|discussion)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c, wsID, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	tasks, err := c.ListTasks()
	if err != nil {
		return cliFail("list: %v", err)
	}
	out := make([]task, 0, len(tasks))
	for _, t := range tasks {
		if *status != "" && t.Status != *status {
			continue
		}
		if *typ != "" && t.Type != *typ {
			continue
		}
		t.Description = ""
		t.AcceptanceCriteria = ""
		out = append(out, t)
	}
	if *asJSON {
		return printCLIJSON(map[string]any{"workspaceId": wsID, "count": len(out), "items": out})
	}
	if len(out) == 0 {
		fmt.Println("(no items)")
		return 0
	}
	for _, t := range out {
		typeCol := t.Type
		if typeCol == "" {
			typeCol = "task"
		}
		fmt.Printf("#%-4d %-10s %-11s %s\n", t.Number, typeCol, t.Status, t.Title)
	}
	return 0
}

func cliGetLike(args []string, graph bool) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default: cwd)")
	_ = fs.Bool("json", false, "machine-readable output (raw daemon JSON)")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("requires <id>\n%s", cliUsage)
	}
	c, wsID, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	if ok, err := c.InWorkspace(id); err != nil {
		return cliFail("%v", err)
	} else if !ok {
		return cliFail("item %s is not in project %s (pass --project or cd into the right project)", id, wsID)
	}
	var st int
	var body []byte
	var err error
	if graph {
		st, body, err = c.GetTaskGraph(id)
	} else {
		st, body, err = c.GetTask(id)
	}
	return emitRaw(st, body, err)
}

func cliCreate(args []string) int {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default: cwd)")
	jsonPayload := fs.String("json", "", "full CreateTaskArgs JSON payload (overlaid by convenience flags)")
	title := fs.String("title", "", "title")
	typ := fs.String("type", "", "task|requirement|bug")
	priority := fs.String("priority", "", "urgent|high|medium|low")
	milestone := fs.String("milestone", "", "milestone name")
	assignee := fs.String("assignee", "", "executing agent type or 'user'")
	acceptance := fs.String("acceptance", "", "acceptance criteria")
	desc := fs.String("description", "", "description / work instruction")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	var a CreateTaskArgs
	if strings.TrimSpace(*jsonPayload) != "" {
		if err := json.Unmarshal([]byte(*jsonPayload), &a); err != nil {
			return cliFail("--json: %v", err)
		}
	}
	// Overlay only explicitly-set convenience flags so they don't clobber --json.
	set := setFlags(fs)
	if set["title"] {
		a.Title = *title
	}
	if set["type"] {
		a.Type = *typ
	}
	if set["priority"] {
		a.Priority = *priority
	}
	if set["milestone"] {
		a.Milestone = *milestone
	}
	if set["assignee"] {
		a.Assignee = *assignee
	}
	if set["acceptance"] {
		a.AcceptanceCriteria = *acceptance
	}
	if set["description"] {
		a.Description = *desc
	}
	if strings.TrimSpace(a.Title) == "" {
		return cliFail("--title is required (or provide it in --json)\n%s", cliUsage)
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	st, resp, err := c.CreateTask(a)
	return emitCreated(st, resp, err)
}

func cliDiscussion(args []string) int {
	fs := flag.NewFlagSet("discussion", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default: cwd)")
	title := fs.String("title", "", "title")
	desc := fs.String("description", "", "body (Markdown)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*title) == "" {
		return cliFail("--title is required\n%s", cliUsage)
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	st, resp, err := c.CreateDiscussion(*title, *desc)
	return emitCreated(st, resp, err)
}

func cliUpdate(args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default: cwd)")
	jsonPayload := fs.String("json", "", "full patch JSON (overlaid by convenience flags)")
	status := fs.String("status", "", "completed|cancelled")
	issueState := fs.String("issue-state", "", "open|closed")
	priority := fs.String("priority", "", "urgent|high|medium|low")
	milestone := fs.String("milestone", "", "milestone name")
	typ := fs.String("type", "", "task|requirement|bug")
	assignee := fs.String("assignee", "", "executing agent type")
	acceptance := fs.String("acceptance", "", "acceptance criteria")
	desc := fs.String("description", "", "description")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("update requires <id>\n%s", cliUsage)
	}
	patch := map[string]json.RawMessage{}
	if strings.TrimSpace(*jsonPayload) != "" {
		if err := json.Unmarshal([]byte(*jsonPayload), &patch); err != nil {
			return cliFail("--json: %v", err)
		}
	}
	set := setFlags(fs)
	overlay := map[string]struct {
		on  bool
		val string
	}{
		"status": {set["status"], *status}, "issueState": {set["issue-state"], *issueState},
		"priority": {set["priority"], *priority}, "milestone": {set["milestone"], *milestone},
		"type": {set["type"], *typ}, "assignee": {set["assignee"], *assignee},
		"acceptanceCriteria": {set["acceptance"], *acceptance}, "description": {set["description"], *desc},
	}
	for key, v := range overlay {
		if v.on {
			b, _ := json.Marshal(v.val)
			patch[key] = b
		}
	}
	// Keep only server-accepted keys.
	filtered := map[string]json.RawMessage{}
	for _, f := range updatableItemFields {
		if v, ok := patch[f]; ok {
			filtered[f] = v
		}
	}
	if len(filtered) == 0 {
		return cliFail("no updatable fields provided\n%s", cliUsage)
	}
	c, wsID, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	if ok, err := c.InWorkspace(id); err != nil {
		return cliFail("%v", err)
	} else if !ok {
		return cliFail("item %s is not in project %s", id, wsID)
	}
	st, resp, err := c.UpdateTask(id, filtered)
	return emitUpdated(id, st, resp, err)
}

func cliCloseReopen(args []string, state string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("close", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default: cwd)")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("requires <id>\n%s", cliUsage)
	}
	c, wsID, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	if ok, err := c.InWorkspace(id); err != nil {
		return cliFail("%v", err)
	} else if !ok {
		return cliFail("item %s is not in project %s", id, wsID)
	}
	sb, _ := json.Marshal(state)
	st, resp, err := c.UpdateTask(id, map[string]json.RawMessage{"issueState": sb})
	return emitUpdated(id, st, resp, err)
}

func cliMilestones(args []string) int {
	if len(args) == 0 {
		return cliFail("milestones <list|create|update>\n%s", cliUsage)
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("milestones list", flag.ContinueOnError)
		project := fs.String("project", "", "project id|name|path")
		_ = fs.Bool("json", false, "raw daemon JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		c, _, code := cliClient(*project)
		if code >= 0 {
			return code
		}
		return emitRaw(c.ListMilestones())
	case "create":
		fs := flag.NewFlagSet("milestones create", flag.ContinueOnError)
		project := fs.String("project", "", "project id|name|path")
		name := fs.String("name", "", "milestone name")
		desc := fs.String("description", "", "description")
		target := fs.String("target-date", "", "RFC3339 target date")
		pred := fs.String("predecessor", "", "predecessor milestone id")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if strings.TrimSpace(*name) == "" {
			return cliFail("--name is required")
		}
		c, _, code := cliClient(*project)
		if code >= 0 {
			return code
		}
		return emitRaw(c.CreateMilestone(CreateMilestoneArgs{Name: *name, Description: *desc, TargetDate: *target, PredecessorID: *pred}))
	case "update":
		id, rest := splitLeadingID(args[1:])
		fs := flag.NewFlagSet("milestones update", flag.ContinueOnError)
		project := fs.String("project", "", "project id|name|path")
		name := fs.String("name", "", "milestone name")
		desc := fs.String("description", "", "description")
		target := fs.String("target-date", "", "RFC3339 target date")
		pred := fs.String("predecessor", "", "predecessor milestone id")
		if err := fs.Parse(rest); err != nil {
			return 1
		}
		if id == "" {
			return cliFail("milestones update requires <id>")
		}
		c, wsID, code := cliClient(*project)
		if code >= 0 {
			return code
		}
		patch := map[string]any{"workspace_id": wsID}
		set := setFlags(fs)
		if set["name"] {
			patch["name"] = *name
		}
		if set["description"] {
			patch["description"] = *desc
		}
		if set["target-date"] {
			patch["targetDate"] = *target
		}
		if set["predecessor"] {
			patch["predecessorId"] = *pred
		}
		if len(patch) == 1 {
			return cliFail("no updatable fields provided")
		}
		return emitRaw(c.UpdateMilestone(id, patch))
	default:
		return cliFail("milestones <list|create|update>")
	}
}

// ── output helpers ───────────────────────────────────────────────────────────

func printCLIJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return cliFail("encode json: %v", err)
	}
	return 0
}

// emitRaw prints a raw daemon JSON body (pretty-printed), or the error.
func emitRaw(status int, body []byte, err error) int {
	if err != nil {
		return cliFail("%v", err)
	}
	if status != 200 {
		return cliFail("request failed (%d): %s", status, strings.TrimSpace(string(body)))
	}
	var pretty any
	if json.Unmarshal(body, &pretty) == nil {
		return printCLIJSON(pretty)
	}
	fmt.Println(string(body))
	return 0
}

func emitCreated(status int, resp []byte, err error) int {
	if err != nil {
		return cliFail("%v", err)
	}
	if status != 200 {
		return cliFail("create failed (%d): %s", status, strings.TrimSpace(string(resp)))
	}
	var created task
	if json.Unmarshal(resp, &created) == nil && created.ID != "" {
		fmt.Printf("created #%d %s: %s\n", created.Number, created.ID, created.Title)
		return 0
	}
	fmt.Println(string(resp))
	return 0
}

func emitUpdated(id string, status int, resp []byte, err error) int {
	if err != nil {
		return cliFail("%v", err)
	}
	if status != 200 {
		return cliFail("update failed (%d): %s", status, strings.TrimSpace(string(resp)))
	}
	fmt.Printf("updated %s\n", id)
	return 0
}

// splitLeadingID pops a leading non-flag positional <id> (Go's flag package
// stops at the first positional, so verbs take the id before the flags).
func splitLeadingID(args []string) (id string, rest []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

// setFlags returns the set of flag names explicitly passed on the command line.
func setFlags(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

// ── workspace + base URL resolution ──────────────────────────────────────────

// resolveBaseURL discovers the running daemon's loopback URL from
// ~/.1agents/daemon.json (honoring ONEAGENTS_HOME), mirroring checkDaemonRunning.
func resolveBaseURL() (string, error) {
	home := os.Getenv("ONEAGENTS_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home dir: %w", err)
		}
		home = h
	}
	path := filepath.Join(home, ".1agents", "daemon.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s (is the 1agents daemon running?): %w", path, err)
	}
	var d struct {
		ListenAddr string `json:"listen_addr"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return "", fmt.Errorf("parse daemon.json: %w", err)
	}
	addr := strings.TrimSpace(d.ListenAddr)
	switch {
	case strings.HasPrefix(addr, ":"):
		addr = "127.0.0.1" + addr
	case strings.HasPrefix(addr, "0.0.0.0:"):
		addr = "127.0.0.1" + strings.TrimPrefix(addr, "0.0.0.0")
	}
	if addr == "" {
		return "", fmt.Errorf("daemon.json has no listen_addr")
	}
	return "http://" + addr, nil
}

type wsRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// resolveWorkspaceID applies the precedence: ONEAGENTS_WORKSPACE_ID env →
// --project flag → current-directory walk-up match against /api/workspace/list.
func resolveWorkspaceID(baseURL, project string) (string, error) {
	if env := strings.TrimSpace(os.Getenv("ONEAGENTS_WORKSPACE_ID")); env != "" {
		return env, nil
	}
	list, err := fetchWorkspaces(baseURL)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(project) != "" {
		np := normalizePath(project)
		for _, w := range list {
			if w.ID == project || w.Name == project || normalizePath(w.Path) == np {
				return w.ID, nil
			}
		}
		return "", fmt.Errorf("no project matches %q (id, name, or path)", project)
	}
	// cwd walk-up
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot get cwd: %w", err)
	}
	byPath := map[string]string{}
	for _, w := range list {
		if w.Path != "" {
			byPath[normalizePath(w.Path)] = w.ID
		}
	}
	dir := normalizePath(cwd)
	for {
		if id, ok := byPath[dir]; ok {
			return id, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("current directory %s is not inside any project — pass --project <id|name|path> or cd into a project directory", cwd)
}

func fetchWorkspaces(baseURL string) ([]wsRecord, error) {
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(baseURL + "/api/workspace/list")
	if err != nil {
		return nil, fmt.Errorf("cannot reach daemon at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("workspace/list failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var list []wsRecord
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse workspace list: %w", err)
	}
	return list, nil
}

// normalizePath canonicalizes a filesystem path for identity comparison: Abs +
// Clean, case-folded on case-insensitive filesystems (darwin/windows). Mirrors
// the workspace handler's normalizeWorkspacePath.
func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs
}
