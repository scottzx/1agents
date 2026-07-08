package sources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGovernanceManifests(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	dir := GovernanceDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `name: contacts
steps:
  - name: gold_feishu_contacts
    source: feishu
    domain: contacts
    output: gold_feishu_contacts
    upstreams: [silver_feishu_users, silver_feishu_messages]
    body: "INSERT INTO gold_feishu_contacts SELECT 1"
  - name: unified_contacts
    source: unified
    domain: contacts
    output: unified_contacts
    upstreams: [silver_icloud_contacts, gold_feishu_contacts]
    script: scripts/contacts_unify.py
    conflict: [entity_key]
    inputSQL: "SELECT 1"
`
	if err := os.WriteFile(filepath.Join(dir, "contacts.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	gms, err := LoadGovernanceManifests()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(gms) != 1 || gms[0].Name != "contacts" || len(gms[0].Steps) != 2 {
		t.Fatalf("parsed = %+v", gms)
	}
	sql, script := gms[0].Steps[0], gms[0].Steps[1]
	if sql.Source != "feishu" || sql.Output != "gold_feishu_contacts" || len(sql.Upstreams) != 2 {
		t.Fatalf("sql step: %+v", sql)
	}
	if script.Script != "scripts/contacts_unify.py" || script.Source != "unified" || len(script.Conflict) != 1 {
		t.Fatalf("script step: %+v", script)
	}
}
