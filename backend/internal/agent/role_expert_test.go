package agent

import "testing"

// TestBuiltinExpertRoles asserts the #139 expert/PMO employee-layer roles load
// as builtins, declare a narrowed toolset (allow/deny), and that every skill
// they name resolves in the skill registry so the role isn't degraded.
func TestBuiltinExpertRoles(t *testing.T) {
	reg := LoadRoles("")

	cases := []struct {
		name       string
		wantAllow  []string
		wantDeny   []string
		wantSkills []string
		wantMcp    []string // empty = none declared
	}{
		{
			name:       "pmo",
			wantAllow:  []string{"Read", "WebSearch"},
			wantDeny:   []string{"Bash", "Write", "Edit"},
			wantSkills: []string{"writing-plans"},
			wantMcp:    []string{"tasks"},
		},
		{
			name:       "content-operator",
			wantAllow:  []string{"Read", "Write", "Edit", "WebSearch"},
			wantDeny:   []string{"Bash"},
			wantSkills: []string{"brainstorming"},
		},
		{
			name:       "growth-strategist",
			wantAllow:  []string{"Read", "WebSearch"},
			wantDeny:   []string{"Bash", "Write", "Edit"},
			wantSkills: []string{"deep-research", "brainstorming"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tpl, ok := reg.Resolve(c.name)
			if !ok {
				t.Fatalf("builtin role %q not resolved", c.name)
			}
			if tpl.Source != "builtin" {
				t.Errorf("Source = %q, want builtin", tpl.Source)
			}
			if tpl.Prompt == "" {
				t.Error("empty prompt")
			}
			if !equalStrings(tpl.Tools.Allow, c.wantAllow) {
				t.Errorf("Tools.Allow = %v, want %v", tpl.Tools.Allow, c.wantAllow)
			}
			if !equalStrings(tpl.Tools.Deny, c.wantDeny) {
				t.Errorf("Tools.Deny = %v, want %v", tpl.Tools.Deny, c.wantDeny)
			}
			if !equalStrings(tpl.Skills, c.wantSkills) {
				t.Errorf("Skills = %v, want %v", tpl.Skills, c.wantSkills)
			}
			if len(tpl.MissingSkills) != 0 {
				t.Errorf("declared skills did not resolve: %v", tpl.MissingSkills)
			}
			if c.wantMcp != nil && !equalStrings(tpl.McpServers, c.wantMcp) {
				t.Errorf("McpServers = %v, want %v", tpl.McpServers, c.wantMcp)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
