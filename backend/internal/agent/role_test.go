package agent

import (
	"encoding/json"
	"reflect"
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
