package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// TestRunReset verifies the reset wipes meta.db data tables and the on-disk
// scratch/knowledge files, re-seeds the default workspace, and never touches the
// relay pairing identity (relay-creds.json / ~/.happy).
func TestRunReset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	oneAgents := filepath.Join(home, ".1agents")

	// ── seed meta.db with a row in every data table ──────────────────────────
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatalf("open meta: %v", err)
	}
	if err := db.EnsureWorkspaceProject(meta.Project{ID: "p1", Name: "Proj", WorkspacePath: "/tmp/p1"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	inbox := meta.NewInboxStore(db)
	if _, err := inbox.Capture(meta.InboxItem{Title: "note", Content: "x"}); err != nil {
		t.Fatalf("seed inbox: %v", err)
	}

	// ── seed file storage (data) + relay identity (must survive) ─────────────
	mustWrite(t, filepath.Join(oneAgents, "devices.json"), `[{"id":"mac"}]`)
	mustWrite(t, filepath.Join(oneAgents, "session_names.json"), `{}`)
	mustWrite(t, filepath.Join(oneAgents, "knowledge", "wiki", "a.md"), "# hi")
	mustWrite(t, filepath.Join(oneAgents, "acpx-state", "s.json"), "{}")
	mustWrite(t, filepath.Join(oneAgents, "projects", "default", "CLAUDE.md"), "old")
	// preserved:
	mustWrite(t, filepath.Join(oneAgents, "relay-creds.json"), `{"token":"keep","secretB64":"keep"}`)
	mustWrite(t, filepath.Join(oneAgents, "daemon.json"), `{"keep":true}`)
	happyKey := filepath.Join(home, ".happy", "access.key")
	mustWrite(t, happyKey, `{"token":"happy-keep"}`)

	reseeded := false
	purged := 0
	summary, err := runReset(
		func() error { reseeded = true; return nil },
		func() (int, error) { purged = 7; return purged, nil },
	)
	if err != nil {
		t.Fatalf("runReset: %v", err)
	}

	// ── meta.db tables empty (default workspace re-seed runs last, so allow it
	// to recreate the "default" project — assert our seeded p1 is gone) ──────
	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	for _, p := range projects {
		if p.ID == "p1" {
			t.Errorf("seeded project p1 survived reset")
		}
	}
	items, err := inbox.List(true)
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("inbox_items not cleared: %d rows", len(items))
	}
	if len(summary.ClearedTables) == 0 {
		t.Error("summary.ClearedTables empty")
	}

	// ── data files gone ──────────────────────────────────────────────────────
	assertMissing(t, filepath.Join(oneAgents, "devices.json"))
	assertMissing(t, filepath.Join(oneAgents, "session_names.json"))
	assertMissing(t, filepath.Join(oneAgents, "knowledge", "wiki", "a.md"))
	assertMissing(t, filepath.Join(oneAgents, "acpx-state", "s.json"))
	assertMissing(t, filepath.Join(oneAgents, "projects", "default", "CLAUDE.md"))

	// ── relay identity preserved ─────────────────────────────────────────────
	assertExists(t, filepath.Join(oneAgents, "relay-creds.json"))
	assertExists(t, filepath.Join(oneAgents, "daemon.json"))
	assertExists(t, happyKey)

	// ── default workspace re-seeded ──────────────────────────────────────────
	if !reseeded || !summary.DefaultSeeded {
		t.Error("default workspace not re-seeded")
	}
	if summary.PurgedProjects != purged {
		t.Errorf("PurgedProjects = %d, want %d", summary.PurgedProjects, purged)
	}
	if !summary.OK {
		t.Error("summary.OK is false")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, but it still exists (err=%v)", path, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s preserved, but: %v", path, err)
	}
}
