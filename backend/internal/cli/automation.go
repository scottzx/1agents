package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/execution"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

const automationPrefix = "automation:"

const automationUsage = `usage:
  1agents automation create --title <name> --instructions <text>
      [--project <id|name|path>] [--profile <profile-id>] [--cwd <path>]
      [--preamble] [--script <relpath>] [--timeout <minutes>]
      [--every-minutes <n>] [--at <RFC3339>] [--run] [--json]
  1agents automation list [--project <id|name|path>] [--json]
  1agents automation get|runs|run|pause|resume <job-id>

Creates a recipe: one task + one ExecutionJob with businessRef automation:<itemId>.
Optional Function preamble (--preamble / --script) runs before ACP.
Requires a running daemon (ONEAGENTS_URL or http://127.0.0.1:38080).`

func runAutomation(args []string) int {
	if len(args) == 0 {
		fmt.Println(automationUsage)
		return 1
	}
	switch args[0] {
	case "create":
		return automationCreate(args[1:])
	case "list":
		return automationList(args[1:])
	case "get", "runs", "run", "pause", "resume":
		id, rest := splitLeadingID(args[1:])
		if id == "" || len(rest) != 0 {
			return fail("automation %s requires <job-id>\n%s", args[0], automationUsage)
		}
		if args[0] == "get" {
			return executionRequest("GET", "/api/execution-jobs/"+id, nil)
		}
		if args[0] == "runs" {
			return executionRequest("GET", "/api/execution-jobs/"+id+"/runs", nil)
		}
		return executionRequest("POST", "/api/execution-jobs/"+id+"/"+args[0], nil)
	case "help", "-h", "--help":
		fmt.Println(automationUsage)
		return 0
	default:
		return fail("unknown automation command %q\n%s", args[0], automationUsage)
	}
}

type automationCreateFlags struct {
	Title        string
	Instructions string
	Project      string
	Profile      string
	Cwd          string
	Preamble     bool
	Script       string
	Timeout      int
	EveryMinutes int
	At           string
	Run          bool
	JSON         bool
}

func parseAutomationCreate(args []string) (automationCreateFlags, error) {
	fs := flag.NewFlagSet("automation create", flag.ContinueOnError)
	var in automationCreateFlags
	fs.StringVar(&in.Title, "title", "", "recipe name")
	fs.StringVar(&in.Instructions, "instructions", "", "ACP prompt / work instruction")
	fs.StringVar(&in.Project, "project", "", "project id|name|path (default: cwd)")
	fs.StringVar(&in.Profile, "profile", "", "agent profile id")
	fs.StringVar(&in.Cwd, "cwd", "", "ACP working directory (default: project path)")
	fs.BoolVar(&in.Preamble, "preamble", false, "run core.script before ACP")
	fs.StringVar(&in.Script, "script", "", "relative python script; implies --preamble")
	fs.IntVar(&in.Timeout, "timeout", 0, "timeout minutes")
	fs.IntVar(&in.EveryMinutes, "every-minutes", 0, "recurring trigger")
	fs.StringVar(&in.At, "at", "", "one-shot RFC3339 trigger")
	fs.BoolVar(&in.Run, "run", false, "run once immediately after create")
	fs.BoolVar(&in.JSON, "json", false, "print created recipe as JSON")
	if err := fs.Parse(args); err != nil {
		return in, err
	}
	return in, validateAutomationCreate(in)
}

func validateAutomationCreate(in automationCreateFlags) error {
	if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Instructions) == "" {
		return fmt.Errorf("--title and --instructions are required")
	}
	if in.EveryMinutes > 0 && strings.TrimSpace(in.At) != "" {
		return fmt.Errorf("use either --every-minutes or --at, not both")
	}
	if in.EveryMinutes < 0 {
		return fmt.Errorf("--every-minutes must be positive")
	}
	if strings.TrimSpace(in.At) != "" {
		if _, err := time.Parse(time.RFC3339, in.At); err != nil {
			return fmt.Errorf("--at must be RFC3339: %w", err)
		}
	}
	return nil
}

func automationCreate(args []string) int {
	in, err := parseAutomationCreate(args)
	if err != nil {
		return fail("%v\n%s", err, automationUsage)
	}
	db, err := openDB()
	if err != nil {
		return fail("open db: %v", err)
	}
	project, err := resolveProjectOrCWD(db, in.Project)
	if err != nil {
		return fail("%v", err)
	}
	cwd := strings.TrimSpace(in.Cwd)
	if cwd == "" {
		cwd = project.WorkspacePath
	}
	usePreamble := in.Preamble || strings.TrimSpace(in.Script) != ""
	script := strings.TrimSpace(in.Script)
	if usePreamble && script == "" {
		script = "automation.py"
	}

	itemRaw, err := daemonRequest("POST", "/api/agent/project-items", map[string]any{
		"workspace_id": project.ID,
		"title":        in.Title,
		"description":  in.Instructions,
		"type":         "task",
		"executor":     "agent",
	})
	if err != nil {
		return fail("create work item: %v", err)
	}
	var item struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(itemRaw, &item); err != nil || item.ID == "" {
		return fail("create work item: invalid response %s", strings.TrimSpace(string(itemRaw)))
	}

	jobBody := map[string]any{
		"projectId":      project.ID,
		"workItemId":     item.ID,
		"businessRef":    automationPrefix + item.ID,
		"executorKind":   "agent",
		"cwd":            cwd,
		"timeoutMinutes": in.Timeout,
		"maxAttempts":    1,
	}
	if strings.TrimSpace(in.Profile) != "" {
		jobBody["profileId"] = in.Profile
	}
	if usePreamble {
		jobBody["preambleFunctionType"] = "core.script"
		jobBody["capabilities"] = []string{"script:" + script}
	}
	jobRaw, err := daemonRequest("POST", "/api/execution-jobs", jobBody)
	if err != nil {
		return fail("create job (item %s already exists): %v", item.ID, err)
	}
	var job execution.Job
	if err := json.Unmarshal(jobRaw, &job); err != nil {
		return fail("create job: invalid response %s", strings.TrimSpace(string(jobRaw)))
	}

	var trigger *execution.Trigger
	if in.EveryMinutes > 0 {
		raw, trigErr := daemonRequest("PUT", "/api/execution-jobs/"+job.ID+"/trigger", map[string]any{
			"kind":          "recurrence",
			"spec":          map[string]any{"everyMinutes": in.EveryMinutes},
			"timezone":      time.Local.String(),
			"misfirePolicy": "skip",
			"overlapPolicy": "forbid",
		})
		if trigErr != nil {
			return fail("set trigger (job %s created): %v", job.ID, trigErr)
		}
		var t execution.Trigger
		_ = json.Unmarshal(raw, &t)
		trigger = &t
	} else if strings.TrimSpace(in.At) != "" {
		raw, trigErr := daemonRequest("PUT", "/api/execution-jobs/"+job.ID+"/trigger", map[string]any{
			"kind":          "at",
			"spec":          map[string]any{"at": in.At},
			"timezone":      time.Local.String(),
			"misfirePolicy": "run_once",
			"overlapPolicy": "forbid",
		})
		if trigErr != nil {
			return fail("set trigger (job %s created): %v", job.ID, trigErr)
		}
		var t execution.Trigger
		_ = json.Unmarshal(raw, &t)
		trigger = &t
	}

	if in.Run {
		if _, runErr := daemonRequest("POST", "/api/execution-jobs/"+job.ID+"/run", nil); runErr != nil {
			return fail("run now (job %s created): %v", job.ID, runErr)
		}
	}

	out := map[string]any{
		"itemId": item.ID, "jobId": job.ID, "projectId": project.ID,
		"businessRef": automationPrefix + item.ID, "title": in.Title,
		"cwd": cwd, "preamble": usePreamble, "script": script, "runRequested": in.Run,
	}
	if trigger != nil {
		out["trigger"] = trigger
	}
	if in.JSON {
		return printJSON(out)
	}
	fmt.Printf("automation %s  item %s  job %s  project %s\n", in.Title, item.ID, job.ID, project.ID)
	if usePreamble {
		fmt.Printf("preamble core.script  script %s\n", script)
	}
	if trigger != nil && trigger.NextRunAt != nil {
		fmt.Printf("next run %s\n", trigger.NextRunAt.Local().Format(time.RFC3339))
	}
	if in.Run {
		fmt.Println("run accepted")
	}
	return 0
}

func automationList(args []string) int {
	fs := flag.NewFlagSet("automation list", flag.ContinueOnError)
	project := fs.String("project", "", "filter by project id|name|path")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	projectID := ""
	if strings.TrimSpace(*project) != "" {
		db, err := openDB()
		if err != nil {
			return fail("open db: %v", err)
		}
		p, err := resolveProjectOrCWD(db, *project)
		if err != nil {
			return fail("%v", err)
		}
		projectID = p.ID
	}
	path := "/api/execution-jobs"
	if projectID != "" {
		path += "?projectId=" + projectID
	}
	raw, err := daemonRequest("GET", path, nil)
	if err != nil {
		return fail("%v", err)
	}
	var payload struct {
		Items []execution.JobDetail `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fail("decode jobs: %v", err)
	}
	recipes := make([]execution.JobDetail, 0, len(payload.Items))
	for _, job := range payload.Items {
		if strings.HasPrefix(job.BusinessRef, automationPrefix) {
			recipes = append(recipes, job)
		}
	}
	if *asJSON {
		return printJSON(map[string]any{"items": recipes})
	}
	if len(recipes) == 0 {
		fmt.Println("no automations")
		return 0
	}
	for _, job := range recipes {
		next := "manual"
		if job.Trigger != nil && job.Trigger.NextRunAt != nil {
			next = job.Trigger.NextRunAt.Local().Format("2006-01-02 15:04")
		}
		preamble := "-"
		if job.PreambleFunctionType != "" {
			preamble = job.PreambleFunctionType
		}
		fmt.Printf("%s  %-8s  preamble=%-12s  next=%s  item=%s\n", job.ID, job.Status, preamble, next, job.WorkItemID)
	}
	return 0
}

func resolveProjectOrCWD(db *meta.DB, key string) (meta.Project, error) {
	if strings.TrimSpace(key) != "" {
		return resolveProject(db, key)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return meta.Project{}, fmt.Errorf("cwd: %w", err)
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return meta.Project{}, err
	}
	projects, err := db.ListProjects()
	if err != nil {
		return meta.Project{}, err
	}
	var best meta.Project
	bestLen := -1
	for _, p := range projects {
		root, err := filepath.Abs(p.WorkspacePath)
		if err != nil || root == "" {
			continue
		}
		if cwd == root || strings.HasPrefix(cwd, root+string(filepath.Separator)) {
			if len(root) > bestLen {
				best, bestLen = p, len(root)
			}
		}
	}
	if bestLen < 0 {
		return meta.Project{}, fmt.Errorf("no project matches cwd %s (pass --project)", cwd)
	}
	return best, nil
}
