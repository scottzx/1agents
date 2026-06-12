// Package cli implements the `1agents project ...` and `1agents task ...`
// subcommands. They write directly to the global metadata database
// (~/.1agents/meta.db, WAL mode) so they work whether or not the daemon is
// running — agents use these to fill in task fields as they work.
// See docs/features/project-model/design.md §4.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Run dispatches args (the daemon binary's positional arguments). It returns
// handled=false when args[0] is not a CLI subcommand, in which case the
// caller falls through to its normal behavior.
func Run(args []string) (handled bool, exitCode int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "project":
		return true, runProject(args[1:])
	case "task":
		return true, runTask(args[1:])
	default:
		return false, 0
	}
}

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	return 1
}

func openDB() (*meta.DB, error) {
	return meta.OpenDefault()
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fail("encode json: %v", err)
	}
	return 0
}

// ── project ─────────────────────────────────────────────────────────────────

const projectUsage = `usage:
  1agents project list [--json]
  1agents project add --name <name> --path <workspace-path> [--id <id>]`

func runProject(args []string) int {
	if len(args) == 0 {
		fmt.Println(projectUsage)
		return 1
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("project list", flag.ContinueOnError)
		asJSON := fs.Bool("json", false, "machine-readable output")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		db, err := openDB()
		if err != nil {
			return fail("open db: %v", err)
		}
		projects, err := db.ListProjects()
		if err != nil {
			return fail("list projects: %v", err)
		}
		if *asJSON {
			return printJSON(projects)
		}
		if len(projects) == 0 {
			fmt.Println("no projects")
			return 0
		}
		for _, p := range projects {
			fmt.Printf("%-32s  %-20s  %-8s  %s\n", p.ID, p.Name, p.Status, p.WorkspacePath)
		}
		return 0

	case "add":
		fs := flag.NewFlagSet("project add", flag.ContinueOnError)
		name := fs.String("name", "", "project display name")
		path := fs.String("path", "", "absolute workspace path")
		id := fs.String("id", "", "explicit project id (defaults to a random id)")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if *name == "" || *path == "" {
			return fail("--name and --path are required\n%s", projectUsage)
		}
		db, err := openDB()
		if err != nil {
			return fail("open db: %v", err)
		}
		pid := *id
		if pid == "" {
			pid = meta.NewID()
		}
		if err := db.EnsureProject(pid, *name, *path); err != nil {
			return fail("add project: %v", err)
		}
		fmt.Printf("project %s added (%s → %s)\n", pid, *name, *path)
		return 0

	default:
		fmt.Println(projectUsage)
		return 1
	}
}

// ── task ────────────────────────────────────────────────────────────────────

const taskUsage = `usage:
  1agents task list   [--project <id|name|path>] [--status <status>] [--label <l>] [--json]
  1agents task add    --project <id|name|path> --title <title>
                      [--desc <md>] [--acceptance <criteria>]
                      [--priority urgent|high|medium|low] [--assignee <agent>]
                      [--labels a,b] [--parent <id>] [--milestone <m>]
                      [--planned-start <t>] [--planned-end <t>]
                      [--depends-on <id,id>] [--recur <rule>] [--max-retries <n>] [--json]
  1agents task show   <id> [--json]
  1agents task update <id> [--title <t>] [--desc <md>] [--status <s>]
                      [--acceptance <criteria>] [--priority <p>] [--assignee <agent>]
                      [--labels a,b] [--parent <id>] [--milestone <m>]
                      [--planned-start <t>] [--planned-end <t>]
                      [--started-at <t>] [--completed-at <t>] [--summary <s>]
                      [--recur <rule>] [--max-retries <n>]
  1agents task close  <id>
  1agents task reopen <id>
  1agents task comment <id> --text <text> [--author <name>]

  times accept RFC3339 (2026-07-01T10:00:00Z), dates (2026-07-01), or "now"
  recur rules: "daily@09:00" | "weekly:1@09:00" (1=Mon) | "monthly:15@09:00" | "none"`

func runTask(args []string) int {
	if len(args) == 0 {
		fmt.Println(taskUsage)
		return 1
	}
	db, err := openDB()
	if err != nil {
		return fail("open db: %v", err)
	}
	store := meta.NewTaskStore(db)

	switch args[0] {
	case "list":
		return taskList(db, store, args[1:])
	case "add":
		return taskAdd(db, store, args[1:])
	case "show":
		return taskShow(store, args[1:])
	case "update":
		return taskUpdate(db, store, args[1:])
	case "close":
		return taskSetState(store, args[1:], meta.IssueClosed)
	case "reopen":
		return taskSetState(store, args[1:], meta.IssueOpen)
	case "comment":
		return taskComment(store, args[1:])
	default:
		fmt.Println(taskUsage)
		return 1
	}
}

// splitLeadingID pops a leading positional <id> off args so that flags
// written after it still parse (Go's flag package stops at the first
// non-flag argument).
func splitLeadingID(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

// resolveProject accepts a project id, display name, or workspace path.
func resolveProject(db *meta.DB, key string) (meta.Project, error) {
	projects, err := db.ListProjects()
	if err != nil {
		return meta.Project{}, err
	}
	for _, p := range projects {
		if p.ID == key || p.Name == key || p.WorkspacePath == key {
			return p, nil
		}
	}
	return meta.Project{}, fmt.Errorf("project %q not found (try: 1agents project list)", key)
}

// parseRecurFlag parses "daily@09:00" / "weekly:1@09:00" / "monthly:15@09:00".
// "none" (or "") returns nil — used by update to clear the rule.
func parseRecurFlag(s string) (*meta.Recurrence, error) {
	if s == "" || s == "none" {
		return nil, nil
	}
	at := ""
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s, at = s[:i], s[i+1:]
	}
	r := &meta.Recurrence{At: at}
	freq := s
	arg := ""
	if i := strings.IndexByte(s, ':'); i >= 0 {
		freq, arg = s[:i], s[i+1:]
	}
	switch freq {
	case "daily":
		r.Freq = "daily"
	case "weekly":
		r.Freq = "weekly"
		if _, err := fmt.Sscanf(arg, "%d", &r.Weekday); err != nil || r.Weekday < 0 || r.Weekday > 6 {
			return nil, fmt.Errorf("weekly needs :<0-6> (0=Sun), got %q", arg)
		}
	case "monthly":
		r.Freq = "monthly"
		if _, err := fmt.Sscanf(arg, "%d", &r.Monthday); err != nil || r.Monthday < 1 || r.Monthday > 31 {
			return nil, fmt.Errorf("monthly needs :<1-31>, got %q", arg)
		}
	default:
		return nil, fmt.Errorf("unknown recurrence %q (daily|weekly:N|monthly:N)", freq)
	}
	if r.At != "" {
		if _, err := time.Parse("15:04", r.At); err != nil {
			return nil, fmt.Errorf("bad time %q (use HH:MM)", r.At)
		}
	}
	return r, nil
}

func parsePriorityFlag(s string) (meta.Priority, error) {
	switch meta.Priority(s) {
	case meta.PriorityUrgent, meta.PriorityHigh, meta.PriorityMedium, meta.PriorityLow:
		return meta.Priority(s), nil
	default:
		return "", fmt.Errorf("invalid --priority %q (urgent|high|medium|low)", s)
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseTimeFlag(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	if s == "now" {
		t := time.Now().UTC()
		return &t, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			t = t.UTC()
			return &t, nil
		}
	}
	return nil, fmt.Errorf("unrecognized time %q (use RFC3339, 2006-01-02, or \"now\")", s)
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func taskList(db *meta.DB, store *meta.TaskStore, args []string) int {
	fs := flag.NewFlagSet("task list", flag.ContinueOnError)
	project := fs.String("project", "", "filter by project id|name|path")
	status := fs.String("status", "", "filter by workflow status")
	label := fs.String("label", "", "filter by label")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	var projects []meta.Project
	if *project != "" {
		p, err := resolveProject(db, *project)
		if err != nil {
			return fail("%v", err)
		}
		projects = []meta.Project{p}
	} else {
		var err error
		projects, err = db.ListProjects()
		if err != nil {
			return fail("list projects: %v", err)
		}
	}

	type row struct {
		Project string `json:"project"`
		meta.Task
	}
	var rows []row
	for _, p := range projects {
		cfg, err := store.Load(p.WorkspacePath)
		if err != nil {
			return fail("load tasks for %s: %v", p.Name, err)
		}
		for _, t := range cfg.Tasks {
			if *status != "" && string(t.Status) != *status {
				continue
			}
			if *label != "" {
				found := false
				for _, l := range t.Labels {
					if l == *label {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			rows = append(rows, row{Project: p.Name, Task: t})
		}
	}
	if *asJSON {
		if rows == nil {
			rows = []row{}
		}
		return printJSON(rows)
	}
	if len(rows) == 0 {
		fmt.Println("no tasks")
		return 0
	}
	fmt.Printf("%-32s  %-8s %-12s %-10s %-6s %-16s %-16s %-14s %s\n",
		"ID", "PRIORITY", "STATUS", "ASSIGNEE", "ISSUE", "PLANNED-START", "PLANNED-END", "PROJECT", "TITLE")
	for _, r := range rows {
		issue := "open"
		if r.IssueState == meta.IssueClosed {
			issue = "closed"
		}
		assignee := r.Assignee
		if assignee == "" {
			assignee = "claudecode"
		}
		title := r.Title
		if r.ParentID != "" {
			title = "└─ " + title
		}
		if len(r.Labels) > 0 {
			title += "  [" + strings.Join(r.Labels, ",") + "]"
		}
		fmt.Printf("%-32s  %-8s %-12s %-10s %-6s %-16s %-16s %-14s %s\n",
			r.ID, r.Priority, r.Status, assignee, issue,
			fmtTimePtr(r.PlannedStart), fmtTimePtr(r.PlannedEnd),
			r.Project, title)
	}
	return 0
}

func taskAdd(db *meta.DB, store *meta.TaskStore, args []string) int {
	fs := flag.NewFlagSet("task add", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (required)")
	title := fs.String("title", "", "task title (required)")
	desc := fs.String("desc", "", "Markdown description (= the agent's work instruction)")
	acceptance := fs.String("acceptance", "", "acceptance criteria the agent self-checks against")
	priority := fs.String("priority", "medium", "urgent|high|medium|low")
	assignee := fs.String("assignee", "", "executing agent type (claudecode/codex/...)")
	labels := fs.String("labels", "", "comma-separated labels")
	parent := fs.String("parent", "", "parent task id (subtasks gate the parent)")
	milestone := fs.String("milestone", "", "milestone name")
	plannedStart := fs.String("planned-start", "", "planned start time (= automation trigger)")
	plannedEnd := fs.String("planned-end", "", "planned end time")
	dependsOn := fs.String("depends-on", "", "comma-separated prerequisite task ids")
	recur := fs.String("recur", "", `recurrence: "daily@09:00" | "weekly:1@09:00" | "monthly:15@09:00"`)
	maxRetries := fs.Int("max-retries", 1, "auto-retry budget on failure")
	asJSON := fs.Bool("json", false, "print the created task as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *project == "" || *title == "" {
		return fail("--project and --title are required\n%s", taskUsage)
	}
	p, err := resolveProject(db, *project)
	if err != nil {
		return fail("%v", err)
	}
	prio, err := parsePriorityFlag(*priority)
	if err != nil {
		return fail("%v", err)
	}
	rec, err := parseRecurFlag(*recur)
	if err != nil {
		return fail("--recur: %v", err)
	}
	ps, err := parseTimeFlag(*plannedStart)
	if err != nil {
		return fail("--planned-start: %v", err)
	}
	pe, err := parseTimeFlag(*plannedEnd)
	if err != nil {
		return fail("--planned-end: %v", err)
	}

	cfg, err := store.Load(p.WorkspacePath)
	if err != nil {
		return fail("load tasks: %v", err)
	}
	now := time.Now().UTC()
	task := meta.Task{
		ID:                 meta.NewID(),
		Title:              *title,
		Description:        *desc,
		AcceptanceCriteria: *acceptance,
		IssueState:         meta.IssueOpen,
		Status:             meta.TaskStatusPending,
		ScheduleType:       meta.ScheduleTypeImmediate,
		Priority:           prio,
		Assignee:           *assignee,
		Labels:             splitCSV(*labels),
		ParentID:           *parent,
		Milestone:          *milestone,
		Recurrence:         rec,
		MaxRetries:         *maxRetries,
		PlannedStart:       ps,
		PlannedEnd:         pe,
		DependsOn:          splitCSV(*dependsOn),
		CreatedAt:          now,
		UpdatedAt:          now,
		Replies:            []meta.Reply{},
		Sessions:           []meta.SessionMetadata{},
	}
	cfg.Tasks = append(cfg.Tasks, task)
	if err := store.Save(p.WorkspacePath, cfg); err != nil {
		return fail("save: %v", err)
	}
	if *asJSON {
		return printJSON(task)
	}
	fmt.Printf("task %s added to %s: %s\n", task.ID, p.Name, task.Title)
	return 0
}

func taskShow(store *meta.TaskStore, args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("task show", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return fail("task show requires exactly one <id>\n%s", taskUsage)
	}
	task, ok, err := store.GetTask(id)
	if err != nil {
		return fail("get task: %v", err)
	}
	if !ok {
		return fail("task %s not found", id)
	}
	if *asJSON {
		return printJSON(task)
	}
	issue := "🔓 open"
	if task.IssueState == meta.IssueClosed {
		issue = "🔒 closed"
	}
	fmt.Printf("%s  [%s] %s\n", task.Title, task.Status, issue)
	fmt.Printf("id:            %s\n", task.ID)
	fmt.Printf("workspace:     %s\n", task.WorkspacePath)
	fmt.Printf("planned:       %s → %s\n", fmtTimePtr(task.PlannedStart), fmtTimePtr(task.PlannedEnd))
	fmt.Printf("actual:        %s → %s\n", fmtTimePtr(task.StartedAt), fmtTimePtr(task.CompletedAt))
	if len(task.DependsOn) > 0 {
		fmt.Printf("depends on:    %s\n", strings.Join(task.DependsOn, ", "))
	}
	if task.Description != "" {
		fmt.Printf("\n%s\n", task.Description)
	}
	if len(task.Replies) > 0 {
		fmt.Printf("\ntimeline (%d):\n", len(task.Replies))
		for i, rp := range task.Replies {
			who := rp.Author.Name
			if who == "" {
				who = rp.Author.Kind
			}
			fmt.Printf("  [%d] %s @ %s\n", i+1, who, rp.CreatedAt.Local().Format("2006-01-02 15:04"))
			for _, line := range strings.Split(rp.Text, "\n") {
				fmt.Printf("      %s\n", line)
			}
		}
	}
	return 0
}

func taskUpdate(db *meta.DB, store *meta.TaskStore, args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("task update", flag.ContinueOnError)
	title := fs.String("title", "", "new title")
	desc := fs.String("desc", "\x00unset", "new Markdown description")
	acceptance := fs.String("acceptance", "\x00unset", "new acceptance criteria")
	status := fs.String("status", "", "workflow status (pending|queued|running|completed|failed|cancelled|blocked)")
	priority := fs.String("priority", "", "urgent|high|medium|low")
	assignee := fs.String("assignee", "\x00unset", "executing agent type")
	labels := fs.String("labels", "\x00unset", "comma-separated labels (replaces)")
	parent := fs.String("parent", "\x00unset", "parent task id ('' to detach)")
	milestone := fs.String("milestone", "\x00unset", "milestone name")
	recur := fs.String("recur", "\x00unset", `recurrence rule ("none" to clear)`)
	maxRetries := fs.Int("max-retries", -1, "auto-retry budget")
	plannedStart := fs.String("planned-start", "", "planned start time")
	plannedEnd := fs.String("planned-end", "", "planned end time")
	startedAt := fs.String("started-at", "", "actual start time")
	completedAt := fs.String("completed-at", "", "actual completion time")
	summary := fs.String("summary", "\x00unset", "result summary")
	dependsOn := fs.String("depends-on", "\x00unset", "comma-separated prerequisite ids (replaces)")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return fail("task update requires exactly one <id>\n%s", taskUsage)
	}

	task, ok, err := store.GetTask(id)
	if err != nil {
		return fail("get task: %v", err)
	}
	if !ok {
		return fail("task %s not found", id)
	}
	cfg, err := store.Load(task.WorkspacePath)
	if err != nil {
		return fail("load tasks: %v", err)
	}

	var target *meta.Task
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == id {
			target = &cfg.Tasks[i]
			break
		}
	}
	if target == nil {
		return fail("task %s not found in workspace config", id)
	}

	if *title != "" {
		target.Title = *title
	}
	if *desc != "\x00unset" {
		target.Description = *desc
	}
	if *status != "" {
		switch meta.TaskStatus(*status) {
		case meta.TaskStatusPending, meta.TaskStatusQueued, meta.TaskStatusRunning,
			meta.TaskStatusCompleted, meta.TaskStatusFailed, meta.TaskStatusCancelled,
			meta.TaskStatusBlocked:
			target.Status = meta.TaskStatus(*status)
		default:
			return fail("invalid --status %q", *status)
		}
	}
	if *summary != "\x00unset" {
		target.Summary = *summary
	}
	if *acceptance != "\x00unset" {
		target.AcceptanceCriteria = *acceptance
	}
	if *priority != "" {
		prio, err := parsePriorityFlag(*priority)
		if err != nil {
			return fail("%v", err)
		}
		target.Priority = prio
	}
	if *assignee != "\x00unset" {
		target.Assignee = *assignee
	}
	if *labels != "\x00unset" {
		target.Labels = splitCSV(*labels)
	}
	if *parent != "\x00unset" {
		target.ParentID = *parent
	}
	if *milestone != "\x00unset" {
		target.Milestone = *milestone
	}
	if *recur != "\x00unset" {
		rec, err := parseRecurFlag(*recur)
		if err != nil {
			return fail("--recur: %v", err)
		}
		target.Recurrence = rec
	}
	if *maxRetries >= 0 {
		target.MaxRetries = *maxRetries
	}
	if *dependsOn != "\x00unset" {
		target.DependsOn = splitCSV(*dependsOn)
	}
	if t, err := parseTimeFlag(*plannedStart); err != nil {
		return fail("--planned-start: %v", err)
	} else if t != nil {
		target.PlannedStart = t
	}
	if t, err := parseTimeFlag(*plannedEnd); err != nil {
		return fail("--planned-end: %v", err)
	} else if t != nil {
		target.PlannedEnd = t
	}
	if t, err := parseTimeFlag(*startedAt); err != nil {
		return fail("--started-at: %v", err)
	} else if t != nil {
		target.StartedAt = t
	}
	if t, err := parseTimeFlag(*completedAt); err != nil {
		return fail("--completed-at: %v", err)
	} else if t != nil {
		target.CompletedAt = t
	}
	target.UpdatedAt = time.Now().UTC()

	if err := store.Save(task.WorkspacePath, cfg); err != nil {
		return fail("save: %v", err)
	}
	fmt.Printf("task %s updated\n", id)
	return 0
}

func taskSetState(store *meta.TaskStore, args []string, state meta.IssueState) int {
	if len(args) != 1 {
		return fail("requires exactly one <id>\n%s", taskUsage)
	}
	if err := store.SetIssueState(args[0], state); err != nil {
		return fail("set issue state: %v", err)
	}
	fmt.Printf("task %s is now %s\n", args[0], state)
	return 0
}

func taskComment(store *meta.TaskStore, args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("task comment", flag.ContinueOnError)
	text := fs.String("text", "", "comment body (required)")
	author := fs.String("author", "cli", "author name")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" || *text == "" {
		return fail("task comment requires <id> and --text\n%s", taskUsage)
	}
	rp, err := store.AppendReply(id, meta.Reply{
		Author: meta.Author{Kind: "user", Name: *author},
		Text:   *text,
		Mode:   meta.ModePureComment,
	})
	if err != nil {
		return fail("append reply: %v", err)
	}
	fmt.Printf("reply %s added to task %s\n", rp.ID, id)
	return 0
}
