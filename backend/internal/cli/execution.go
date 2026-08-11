package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const executionUsage = `usage:
  1agents execution create --project <id> --item <work-item-id>
      [--executor agent|function|human] [--profile <profile-id>] [--legacy-agent <agent>]
      [--function <type>] [--cwd <path>] [--timeout <minutes>] [--max-attempts <n>]
  1agents execution get|runs <job-id>
  1agents execution pause|resume|archive|run <job-id>
  1agents execution trigger <job-id> --kind at|recurrence --spec '<json>'
  1agents execution trigger-delete <job-id>

Set ONEAGENTS_URL to override http://127.0.0.1:38080. This command requires a running daemon.`

func runExecution(args []string) int {
	if len(args) == 0 {
		fmt.Println(executionUsage)
		return 1
	}
	switch args[0] {
	case "create":
		return executionCreate(args[1:])
	case "get":
		id, rest := splitLeadingID(args[1:])
		if id == "" || len(rest) != 0 {
			return fail("execution get requires <job-id>\n%s", executionUsage)
		}
		return executionRequest(http.MethodGet, "/api/execution-jobs/"+id, nil)
	case "runs":
		id, rest := splitLeadingID(args[1:])
		if id == "" || len(rest) != 0 {
			return fail("execution runs requires <job-id>\n%s", executionUsage)
		}
		return executionRequest(http.MethodGet, "/api/execution-jobs/"+id+"/runs", nil)
	case "pause", "resume", "archive", "run":
		id, rest := splitLeadingID(args[1:])
		if id == "" || len(rest) != 0 {
			return fail("execution %s requires <job-id>\n%s", args[0], executionUsage)
		}
		return executionRequest(http.MethodPost, "/api/execution-jobs/"+id+"/"+args[0], nil)
	case "trigger":
		return executionTrigger(args[1:])
	case "trigger-delete":
		id, rest := splitLeadingID(args[1:])
		if id == "" || len(rest) != 0 {
			return fail("execution trigger-delete requires <job-id>\n%s", executionUsage)
		}
		return executionRequest(http.MethodDelete, "/api/execution-jobs/"+id+"/trigger", nil)
	case "help", "-h", "--help":
		fmt.Println(executionUsage)
		return 0
	default:
		return fail("unknown execution command %q\n%s", args[0], executionUsage)
	}
}

func executionCreate(args []string) int {
	fs := flag.NewFlagSet("execution create", flag.ContinueOnError)
	project := fs.String("project", "", "project id")
	item := fs.String("item", "", "work item id")
	executor := fs.String("executor", "agent", "agent|function|human")
	profile := fs.String("profile", "", "agent profile id")
	legacyAgent := fs.String("legacy-agent", "", "legacy agent type")
	functionType := fs.String("function", "", "function type")
	cwd := fs.String("cwd", "", "working directory")
	timeout := fs.Int("timeout", 0, "timeout in minutes")
	maxAttempts := fs.Int("max-attempts", 1, "maximum attempts")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *project == "" || *item == "" {
		return fail("--project and --item are required\n%s", executionUsage)
	}
	return executionRequest(http.MethodPost, "/api/execution-jobs", map[string]any{
		"projectId": *project, "workItemId": *item, "executorKind": *executor,
		"profileId": *profile, "legacyAgentType": *legacyAgent, "functionType": *functionType,
		"cwd": *cwd, "timeoutMinutes": *timeout, "maxAttempts": *maxAttempts,
	})
}

func executionTrigger(args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("execution trigger", flag.ContinueOnError)
	kind := fs.String("kind", "", "at|recurrence")
	spec := fs.String("spec", "", "trigger JSON")
	timezone := fs.String("timezone", "", "IANA timezone")
	misfire := fs.String("misfire", "skip", "skip|run_once")
	overlap := fs.String("overlap", "forbid", "forbid|allow")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" || *kind == "" || *spec == "" {
		return fail("trigger requires <job-id>, --kind, and --spec\n%s", executionUsage)
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(*spec), &raw); err != nil {
		return fail("--spec must be JSON: %v", err)
	}
	return executionRequest(http.MethodPut, "/api/execution-jobs/"+id+"/trigger", map[string]any{
		"kind": *kind, "spec": raw, "timezone": *timezone, "misfirePolicy": *misfire, "overlapPolicy": *overlap,
	})
}

func executionRequest(method, path string, body any) int {
	baseURL := strings.TrimRight(os.Getenv("ONEAGENTS_URL"), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:38080"
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fail("encode request: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		return fail("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fail("call daemon: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fail("daemon returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if len(data) > 0 {
		fmt.Print(string(data))
	}
	return 0
}
