package agent

import "testing"

// The 自动调研管线 (#190) ships a builtin 市场分析师 role to run L2 deep research.
// It must load as a builtin and its declared skills (deep-research / design-shotgun,
// both absorbed in #188) must resolve so the role isn't degraded.
func TestBuiltinMarketAnalystRole(t *testing.T) {
	reg := LoadRoles("")
	tpl, ok := reg.Resolve("market-analyst")
	if !ok {
		t.Fatal("builtin market-analyst role not resolved")
	}
	if tpl.Source != "builtin" {
		t.Errorf("Source = %q, want builtin", tpl.Source)
	}
	if tpl.Prompt == "" {
		t.Error("empty prompt")
	}
	want := map[string]bool{"deep-research": true, "design-shotgun": true}
	for _, s := range tpl.Skills {
		delete(want, s)
	}
	if len(want) != 0 {
		t.Errorf("role missing declared skills: %v", want)
	}
	if len(tpl.MissingSkills) != 0 {
		t.Errorf("declared skills did not resolve: %v", tpl.MissingSkills)
	}
}
