package ingest

import "testing"

// TestBuildDAG checks nodes/edges derivation: step outputs are marked isStep,
// pure upstream tables are leaf nodes, and every upstream→output pair is an edge.
func TestBuildDAG(t *testing.T) {
	steps := []govStep{
		{Name: "gold_feishu_contacts", Output: "gold_feishu_contacts", Domain: "contacts",
			Upstreams: []string{"silver_feishu_users", "silver_feishu_messages"}},
		{Name: "unified_contacts", Output: "unified_contacts", Domain: "contacts",
			Upstreams: []string{"silver_icloud_contacts", "gold_feishu_contacts"}},
	}
	nodes, edges := buildDAG(steps)

	isStep := map[string]bool{}
	for _, n := range nodes {
		isStep[n.Table] = n.IsStep
	}
	// Step outputs marked; leaf source tables not.
	if !isStep["gold_feishu_contacts"] || !isStep["unified_contacts"] {
		t.Fatalf("step outputs should be isStep: %+v", nodes)
	}
	if isStep["silver_feishu_users"] || isStep["silver_icloud_contacts"] {
		t.Fatalf("leaf source tables should not be isStep: %+v", nodes)
	}
	// gold_feishu_contacts is both a step output AND unified_contacts' upstream —
	// one node, marked isStep, with a downstream edge.
	if len(nodes) != 5 { // 3 silver leaves + 2 step outputs
		t.Fatalf("nodes = %d, want 5: %+v", len(nodes), nodes)
	}
	want := map[string]bool{
		"silver_feishu_users>gold_feishu_contacts":    false,
		"silver_feishu_messages>gold_feishu_contacts": false,
		"silver_icloud_contacts>unified_contacts":     false,
		"gold_feishu_contacts>unified_contacts":       false,
	}
	for _, e := range edges {
		want[e.From+">"+e.To] = true
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("missing edge %s in %+v", k, edges)
		}
	}
}
