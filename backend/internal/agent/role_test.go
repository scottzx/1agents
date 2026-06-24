package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// builtinPM parses the embedded builtin "pm" role template.
func builtinPM(t *testing.T) *RoleTemplate {
	t.Helper()
	raw, err := builtinRolesFS.ReadFile("roles/pm.md")
	if err != nil {
		t.Fatalf("read embedded pm.md: %v", err)
	}
	tpl, err := parseRoleMarkdown(raw)
	if err != nil {
		t.Fatalf("parse pm.md: %v", err)
	}
	return tpl
}

func TestParseBuiltinPM(t *testing.T) {
	tpl := builtinPM(t)
	if tpl.Name != "pm" {
		t.Errorf("Name = %q, want %q", tpl.Name, "pm")
	}
	if got, want := tpl.McpServers, []string{"tasks"}; !reflect.DeepEqual(got, want) {
		t.Errorf("McpServers = %v, want %v", got, want)
	}
	if tpl.Engine != "claude-code" {
		t.Errorf("Engine = %q, want %q", tpl.Engine, "claude-code")
	}
	if tpl.Prompt == "" {
		t.Error("Prompt is empty")
	}
}

// TestRenderPromptMatchesHardcoded is the strongest "behavior unchanged"
// guarantee: the template-rendered PM prompt must be byte-identical to the
// hardcoded buildPMSystemPrompt it replaces.
func TestRenderPromptMatchesHardcoded(t *testing.T) {
	tpl := builtinPM(t)
	const projectName, workspaceID = "示例项目", "ws-abc-123"
	got := renderRolePrompt(tpl, projectName, workspaceID)
	want := buildPMSystemPrompt(projectName, workspaceID)
	if got != want {
		t.Errorf("rendered prompt differs from buildPMSystemPrompt\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestMcpServersFromRoleMatchesHardcoded ensures the template-driven MCP config
// equals the hardcoded buildPMMcpServers (compared structurally — JSON key
// order isn't guaranteed).
func TestMcpServersFromRoleMatchesHardcoded(t *testing.T) {
	tpl := builtinPM(t)
	h := &Handler{selfBaseURL: "http://127.0.0.1:9999"}
	const workspaceID = "ws-abc-123"

	gotRaw := h.buildMcpServersFromRole(tpl, workspaceID, "")
	wantRaw := h.buildPMMcpServers(workspaceID)
	if gotRaw == nil || wantRaw == nil {
		t.Fatalf("nil mcp config: got=%v want=%v", gotRaw, wantRaw)
	}

	var got, want any
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mcp config differs\ngot:  %s\nwant: %s", gotRaw, wantRaw)
	}
}

// envValue extracts an env var's value from a buildTasksMcpServer entry.
func envValue(srv map[string]any, name string) (string, bool) {
	env, _ := srv["env"].([]map[string]string)
	for _, e := range env {
		if e["name"] == name {
			return e["value"], true
		}
	}
	return "", false
}

// TestTasksMcpServerTaskLock: a non-empty taskID injects ONEAGENTS_TASK_ID;
// an empty taskID (PM/project-wide) leaves it out.
func TestTasksMcpServerTaskLock(t *testing.T) {
	h := &Handler{selfBaseURL: "http://127.0.0.1:9999"}

	locked := h.buildTasksMcpServer("ws1", "t42")
	if v, ok := envValue(locked, "ONEAGENTS_TASK_ID"); !ok || v != "t42" {
		t.Errorf("locked: ONEAGENTS_TASK_ID = %q (ok=%v), want t42", v, ok)
	}
	if v, _ := envValue(locked, "ONEAGENTS_WORKSPACE_ID"); v != "ws1" {
		t.Errorf("locked: ONEAGENTS_WORKSPACE_ID = %q, want ws1", v)
	}

	unlocked := h.buildTasksMcpServer("ws1", "")
	if _, ok := envValue(unlocked, "ONEAGENTS_TASK_ID"); ok {
		t.Error("unlocked: ONEAGENTS_TASK_ID should be absent for project-wide PM")
	}
}

// TestBuiltinExecutorVerifierTemplates: both task-bound role templates parse,
// carry the tasks MCP, and use placeholders renderable by renderRolePrompt.
func TestBuiltinExecutorVerifierTemplates(t *testing.T) {
	for _, name := range []string{"executor", "verifier"} {
		reg := LoadRoles("")
		tpl, ok := reg.Resolve(name)
		if !ok {
			t.Fatalf("builtin %q not resolved", name)
		}
		if tpl.Source != "builtin" || tpl.Name != name {
			t.Errorf("%s: Source=%q Name=%q", name, tpl.Source, tpl.Name)
		}
		if len(tpl.McpServers) != 1 || tpl.McpServers[0] != "tasks" {
			t.Errorf("%s: McpServers=%v, want [tasks]", name, tpl.McpServers)
		}
		if tpl.Prompt == "" {
			t.Errorf("%s: empty prompt", name)
		}
	}
}

// TestRoleTemplateName maps role codes to template names.
func TestRoleTemplateName(t *testing.T) {
	cases := map[string]string{
		"pm":       "pm",
		"pmo":      "pm",
		"executor": "executor",
		"verifier": "verifier",
		"general":  "",
		"auto":     "",
	}
	for role, want := range cases {
		if got := roleTemplateName(role); got != want {
			t.Errorf("roleTemplateName(%q) = %q, want %q", role, got, want)
		}
	}
}

// TestRoleInjectionTaskLock: an executor/verifier injection binds the tasks MCP
// to the given task_id (ONEAGENTS_TASK_ID present), and an unknown role yields
// ok=false.
func TestRoleInjectionTaskLock(t *testing.T) {
	h := &Handler{selfBaseURL: "http://127.0.0.1:9999"}

	for _, role := range []string{"executor", "verifier"} {
		prompt, mcp, ok := h.roleInjection(role, "", "ws1", "task-77")
		if !ok {
			t.Fatalf("%s: roleInjection ok=false", role)
		}
		if prompt == "" {
			t.Errorf("%s: empty persona", role)
		}
		if mcp == nil {
			t.Fatalf("%s: nil mcp", role)
		}
		var arr []map[string]any
		if err := json.Unmarshal(mcp, &arr); err != nil || len(arr) != 1 {
			t.Fatalf("%s: mcp decode: %v (%s)", role, err, mcp)
		}
		raw, _ := json.Marshal(arr[0])
		if !strings.Contains(string(raw), "ONEAGENTS_TASK_ID") || !strings.Contains(string(raw), "task-77") {
			t.Errorf("%s: tasks MCP not locked to task-77: %s", role, raw)
		}
	}

	if _, _, ok := h.roleInjection("general", "", "ws1", "task-77"); ok {
		t.Error("roleInjection for unknown role should be ok=false")
	}
}

func TestEngineToAgentType(t *testing.T) {
	cases := map[string]AgentType{
		"claude-code": AgentTypeClaudecode,
		"claudecode":  AgentTypeClaudecode,
		"codex":       AgentTypeCodex,
		"gemini":      AgentTypeGemini,
		"":            DefaultAgentType,
		"weird-thing": AgentType("weird-thing"),
	}
	for in, want := range cases {
		if got := engineToAgentType(in); got != want {
			t.Errorf("engineToAgentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseRoleMarkdownErrors(t *testing.T) {
	cases := map[string][]byte{
		"no frontmatter": []byte("just a body, no fences"),
		"unterminated":   []byte("---\nname: x\nstill open"),
		"missing name":   []byte("---\ndescription: x\n---\nbody"),
	}
	for name, raw := range cases {
		if _, err := parseRoleMarkdown(raw); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

// TestDegradeUnknownEngine: a template with an unknown engine loads but is
// flagged unavailable rather than crashing the registry.
func TestDegradeUnknownEngine(t *testing.T) {
	tpl, err := parseRoleMarkdown([]byte("---\nname: ghost\nengine: nonexistent-engine\n---\nbody"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	markRoleAvailability(tpl)
	if tpl.Available {
		t.Error("expected Available=false for unknown engine")
	}
	if tpl.Unavailable == "" {
		t.Error("expected an Unavailable reason")
	}
}

// TestLoadRolesAlwaysHasBuiltinPM: the builtin pm template resolves even with
// no user/project dirs present.
func TestLoadRolesAlwaysHasBuiltinPM(t *testing.T) {
	reg := LoadRoles("")
	tpl, ok := reg.Resolve("pm")
	if !ok {
		t.Fatal("builtin pm not resolved")
	}
	if tpl.Source != "builtin" {
		t.Errorf("Source = %q, want builtin", tpl.Source)
	}
}
