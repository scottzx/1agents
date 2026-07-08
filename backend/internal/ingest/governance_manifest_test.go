package ingest

import "testing"

// TestAddGovernanceManifest covers the standalone-governance hot-add: a valid
// manifest registers its steps, an invalid one is rejected, and a re-add with the
// same step name replaces (not duplicates) it.
func TestAddGovernanceManifest(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	h := &Handler{}

	const yaml = `
name: mydag
steps:
  - name: gold_demo
    upstreams: [silver_feishu_users]
    output: gold_demo
    domain: contacts
    createSQL: "CREATE TABLE IF NOT EXISTS gold_demo (id TEXT PRIMARY KEY, updated_at INTEGER)"
    body: "INSERT INTO gold_demo (id, updated_at) SELECT external_id, updated_at FROM silver_feishu_users WHERE true ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at"
    incremental: { table: silver_feishu_users, column: updated_at }
`
	gm, err := h.AddGovernanceManifest(addGovernanceReq{YAML: yaml})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if gm.Name != "mydag" || len(h.manifestGold) != 1 {
		t.Fatalf("registered wrong: name=%s gold=%d", gm.Name, len(h.manifestGold))
	}

	// Re-add (idempotent): same step name must replace, not duplicate.
	if _, err := h.AddGovernanceManifest(addGovernanceReq{YAML: yaml}); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if len(h.manifestGold) != 1 {
		t.Fatalf("re-add duplicated steps: gold=%d", len(h.manifestGold))
	}

	// Invalid: a step with neither body nor script is rejected.
	_, err = h.AddGovernanceManifest(addGovernanceReq{YAML: "name: bad\nsteps:\n  - name: x\n    output: y\n"})
	if err == nil {
		t.Fatal("expected validation error for step with no body/script")
	}

	// A script step's file is written into the governance dir and its step registers.
	const scriptYAML = `
name: scriptdag
steps:
  - name: gold_script
    upstreams: [silver_icloud_contacts]
    output: gold_script
    domain: contacts
    inputSQL: "SELECT external_id, updated_at FROM silver_icloud_contacts WHERE updated_at > :since"
    script: scripts/demo.py
    conflict: [id]
    incremental: { column: updated_at }
`
	before := len(h.manifestScript)
	if _, err := h.AddGovernanceManifest(addGovernanceReq{
		YAML:    scriptYAML,
		Scripts: map[string]string{"scripts/demo.py": "import sys\n"},
	}); err != nil {
		t.Fatalf("add script dag: %v", err)
	}
	if len(h.manifestScript) != before+1 {
		t.Fatalf("script step not registered: %d", len(h.manifestScript))
	}
	// An unsafe script path is rejected.
	if _, err := h.AddGovernanceManifest(addGovernanceReq{
		YAML:    scriptYAML,
		Scripts: map[string]string{"../evil.py": "x"},
	}); err == nil {
		t.Fatal("expected rejection of unsafe script path")
	}
}
