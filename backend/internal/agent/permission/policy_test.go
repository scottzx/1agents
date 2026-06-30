package permission

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestModeIsValid(t *testing.T) {
	cases := map[Mode]bool{
		ModeApproveReads: true,
		ModeApproveAll:   true,
		ModeDenyAll:      true,
		ModeAuto:         true,
		"":               false,
		"yolo":           false,
		"Auto":           false, // case-sensitive
	}
	for m, want := range cases {
		if got := m.IsValid(); got != want {
			t.Errorf("Mode(%q).IsValid() = %v, want %v", m, got, want)
		}
	}
}

func TestMcpServerOf(t *testing.T) {
	cases := map[string]string{
		"mcp__tasks__create_task": "tasks",
		"mcp__github__list_prs":    "github",
		"mcp__tasks":               "tasks", // no tool segment
		"Read":                     "",
		"":                         "",
	}
	for in, want := range cases {
		if got := mcpServerOf(in); got != want {
			t.Errorf("mcpServerOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEvaluate_CoarseModes(t *testing.T) {
	read := Request{ToolName: "Read"}
	write := Request{ToolName: "Write"}

	if got := Evaluate(ModeApproveAll, write); got != DecisionAllow {
		t.Errorf("approve-all/Write = %v, want allow", got)
	}
	if got := Evaluate(ModeDenyAll, read); got != DecisionDeny {
		t.Errorf("deny-all/Read = %v, want deny", got)
	}
	if got := Evaluate(ModeApproveReads, read); got != DecisionAllow {
		t.Errorf("approve-reads/Read = %v, want allow", got)
	}
	if got := Evaluate(ModeApproveReads, write); got != DecisionPrompt {
		t.Errorf("approve-reads/Write = %v, want prompt", got)
	}
}

func TestEvaluate_UnknownModePrompts(t *testing.T) {
	if got := Evaluate(Mode("nonsense"), Request{ToolName: "Read"}); got != DecisionPrompt {
		t.Errorf("unknown mode = %v, want prompt", got)
	}
	if got := Evaluate(Mode(""), Request{ToolName: "Read"}); got != DecisionPrompt {
		t.Errorf("empty mode = %v, want prompt", got)
	}
}

func TestEvaluateAuto(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		want Decision
	}{
		{
			name: "context-locked tasks server auto-allows (the #63 PM case)",
			req:  Request{ToolName: "mcp__tasks__create_task"},
			want: DecisionAllow,
		},
		{
			name: "context-locked server via explicit ServerName",
			req:  Request{ToolName: "create_task", ServerName: "tasks"},
			want: DecisionAllow,
		},
		{
			name: "high-risk built-in Bash prompts",
			req:  Request{ToolName: "Bash"},
			want: DecisionPrompt,
		},
		{
			name: "high-risk Write prompts even with a (untrusted) readOnlyHint=true",
			req:  Request{ToolName: "Write", Annotations: Annotations{ReadOnlyHint: boolPtr(true)}},
			want: DecisionPrompt,
		},
		{
			name: "read-only built-in auto-allows",
			req:  Request{ToolName: "Read"},
			want: DecisionAllow,
		},
		{
			name: "readOnlyHint=true auto-allows an unknown tool",
			req:  Request{ToolName: "mcp__github__get_issue", Annotations: Annotations{ReadOnlyHint: boolPtr(true)}},
			want: DecisionAllow,
		},
		{
			name: "destructiveHint=true prompts",
			req:  Request{ToolName: "mcp__github__delete_repo", Annotations: Annotations{DestructiveHint: boolPtr(true)}},
			want: DecisionPrompt,
		},
		{
			name: "idempotentHint=true (reversible) auto-allows",
			req:  Request{ToolName: "mcp__github__set_label", Annotations: Annotations{IdempotentHint: boolPtr(true)}},
			want: DecisionAllow,
		},
		{
			name: "destructive beats idempotent",
			req:  Request{ToolName: "mcp__x__op", Annotations: Annotations{DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true)}},
			want: DecisionPrompt,
		},
		{
			name: "unknown tool with no hints defaults to prompt",
			req:  Request{ToolName: "mcp__unknown__do_thing"},
			want: DecisionPrompt,
		},
		{
			name: "readOnlyHint=false does not auto-allow",
			req:  Request{ToolName: "mcp__x__op", Annotations: Annotations{ReadOnlyHint: boolPtr(false)}},
			want: DecisionPrompt,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Evaluate(ModeAuto, c.req); got != c.want {
				t.Errorf("Evaluate(auto, %+v) = %v, want %v", c.req, got, c.want)
			}
		})
	}
}

func TestDecisionString(t *testing.T) {
	cases := map[Decision]string{
		DecisionAllow:  "allow",
		DecisionDeny:   "deny",
		DecisionPrompt: "prompt",
	}
	for d, want := range cases {
		if got := d.String(); got != want {
			t.Errorf("Decision(%d).String() = %q, want %q", d, got, want)
		}
	}
}
