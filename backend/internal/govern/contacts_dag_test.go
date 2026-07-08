package govern

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
)

// TestContactsUnificationDAG exercises the standalone cross-source governance DAG:
// ① SQL step unions the Feishu user list (二级用户 ∪ 群消息发送者 兜底) into
// gold_feishu_contacts; ② Python step unions Apple ∪ Feishu into unified_contacts,
// merging Apple contacts that share a phone. Proves the DAG expresses the pipeline.
func TestContactsUnificationDAG(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	st, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db := st.SQL()

	// The Feishu/Apple silver tables already exist (built-in DDL applied by Open);
	// insert into them with explicit columns (the rest carry NOT NULL defaults).
	// Feishu: two known users; a message from a third sender NOT in the user table.
	mustExec(t, db, `INSERT INTO silver_feishu_users (source, account_id, external_id, name, deleted, updated_at) VALUES
		('feishu','d','ou_a','阿伟',0,100),('feishu','d','ou_b','小美',0,100)`)
	mustExec(t, db, `INSERT INTO silver_feishu_messages (source, account_id, external_id, sender_open_id, updated_at) VALUES
		('feishu','d','m1','ou_a',100),('feishu','d','m2','ou_c',100)`) // ou_c only seen in a message

	// Apple: two contacts sharing a phone (should merge) + one distinct.
	mustExec(t, db, `INSERT INTO silver_icloud_contacts (source, account_id, external_id, full_name, phones, deleted, updated_at) VALUES
		('icloud','d','ap1','老王','["+86 138-0000-1111"]',0,100),
		('icloud','d','ap2','王总','["13800001111"]',0,100),
		('icloud','d','ap3','独立联系人','["13900002222"]',0,100)`)

	// ① Feishu union → gold_feishu_contacts (full recompute: no incremental).
	feishuStep := SQLStep{
		Name: "gold_feishu_contacts", Upstreams: []string{"silver_feishu_users", "silver_feishu_messages"},
		Output: "gold_feishu_contacts",
		CreateSQL: `CREATE TABLE IF NOT EXISTS gold_feishu_contacts (
			source TEXT DEFAULT 'feishu', external_id TEXT PRIMARY KEY, name TEXT DEFAULT '', updated_at INTEGER DEFAULT 0)`,
		Body: `INSERT INTO gold_feishu_contacts (source, external_id, name, updated_at)
			SELECT 'feishu', external_id, name, updated_at FROM silver_feishu_users WHERE deleted=0
			UNION
			SELECT 'feishu', sender_open_id, '', 0 FROM silver_feishu_messages
				WHERE sender_open_id<>'' AND sender_open_id NOT IN (SELECT external_id FROM silver_feishu_users WHERE deleted=0)
			ON CONFLICT(external_id) DO UPDATE SET
				name=CASE WHEN excluded.name<>'' THEN excluded.name ELSE gold_feishu_contacts.name END`,
	}
	if _, err := RunSQLStep(st, feishuStep); err != nil {
		t.Fatalf("feishu step: %v", err)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM gold_feishu_contacts`); n != 3 { // ou_a, ou_b, ou_c
		t.Fatalf("gold_feishu_contacts = %d, want 3 (2 users + 1 message-only sender)", n)
	}

	// ② Apple ∪ Feishu → unified_contacts via the real example Python script.
	unifyStep := ScriptStep{
		Name: "unified_contacts", Upstreams: []string{"silver_icloud_contacts", "gold_feishu_contacts"},
		Output: "unified_contacts", Conflict: []string{"entity_key"},
		Script: "../../examples/governance/scripts/contacts_unify.py",
		InputSQL: `SELECT 'apple' AS src, external_id, full_name AS name, phones FROM silver_icloud_contacts WHERE deleted=0
			UNION ALL
			SELECT 'feishu' AS src, external_id, name AS name, '[]' AS phones FROM gold_feishu_contacts`,
		CreateSQL: `CREATE TABLE IF NOT EXISTS unified_contacts (
			entity_key TEXT PRIMARY KEY, external_id TEXT DEFAULT '', source TEXT DEFAULT 'unified',
			name TEXT DEFAULT '', sources TEXT DEFAULT '[]', phones TEXT DEFAULT '[]',
			apple_ids TEXT DEFAULT '[]', feishu_open_ids TEXT DEFAULT '[]', source_count INTEGER DEFAULT 0)`,
	}
	if _, err := RunScriptStep(st, unifyStep); err != nil {
		t.Fatalf("unify step: %v", err)
	}

	// Apple ap1+ap2 share a phone → 1 entity; ap3 → 1; 3 feishu (no phone) → 3. Total 5.
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM unified_contacts`); n != 5 {
		t.Fatalf("unified_contacts = %d, want 5", n)
	}
	// The merged Apple entity carries both apple ids.
	var appleIDs string
	if err := db.QueryRow(`SELECT apple_ids FROM unified_contacts WHERE entity_key='phone:13800001111'`).Scan(&appleIDs); err != nil {
		t.Fatalf("merged apple entity missing: %v", err)
	}
	if !strings.Contains(appleIDs, "ap1") || !strings.Contains(appleIDs, "ap2") {
		t.Fatalf("merged apple_ids = %s, want both ap1+ap2", appleIDs)
	}
}
