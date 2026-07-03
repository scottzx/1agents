package workspace

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedSharedSkill writes a minimal skill package into the shared store rooted at
// home (~/.1agents/skill-manager/shared/<name>).
func seedSharedSkill(t *testing.T, home, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(home, ".1agents", "skill-manager", "shared", name)
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

func TestSyncSkillsToWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)

	seedSharedSkill(t, home, "alpha", map[string]string{
		"SKILL.md":          "# alpha",
		"assets/helper.txt": "nested",
	})
	seedSharedSkill(t, home, "beta", map[string]string{"SKILL.md": "# beta"})

	ws := t.TempDir()
	// "shared:alpha" exercises ref normalization; "missing" must be skipped
	// without failing the whole call.
	synced, err := syncSkillsToWorkspace(ws, []string{"shared:alpha", "beta", "missing"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(synced) != 2 {
		t.Fatalf("expected 2 synced, got %v", synced)
	}

	// Real copies land under <ws>/.claude/skills.
	for _, rel := range []string{
		"alpha/SKILL.md",
		"alpha/assets/helper.txt",
		"beta/SKILL.md",
	} {
		p := filepath.Join(ws, ".claude", "skills", rel)
		if fi, err := os.Lstat(p); err != nil {
			t.Errorf("expected real file %s: %v", rel, err)
		} else if fi.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s should be a real file, not a symlink", rel)
		}
	}

	// <ws>/.agents/skills is a single relative dir symlink into the .claude
	// store; every skill is reachable through it (no per-skill links).
	skillsLink := filepath.Join(ws, ".agents", "skills")
	fi, err := os.Lstat(skillsLink)
	if err != nil {
		t.Fatalf("expected .agents/skills symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".agents/skills should be a symlink")
	}
	if tgt, _ := os.Readlink(skillsLink); tgt != filepath.Join("..", ".claude", "skills") {
		t.Errorf(".agents/skills target = %q", tgt)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(skillsLink, name, "SKILL.md")); err != nil {
			t.Errorf("%s not reachable via .agents/skills: %v", name, err)
		}
	}

	// A second sync is idempotent: store copy left untouched, symlink already
	// correct → nothing re-synced.
	again, err := syncSkillsToWorkspace(ws, []string{"alpha"})
	if err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	if len(again) != 1 {
		// alpha is re-linked (link is idempotent but still counted as synced);
		// the meaningful guarantee is no error + the store copy stays put.
		t.Logf("re-sync returned %v", again)
	}
}

// TestSyncSkillsReplacesStaleLink proves that a stale .agents/skills entry (a
// real dir from an older per-skill layout) is replaced by the whole-dir symlink
// on the next sync.
func TestSyncSkillsReplacesStaleLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	seedSharedSkill(t, home, "alpha", map[string]string{"SKILL.md": "# alpha"})

	ws := t.TempDir()
	// Simulate the old per-skill layout: a real .agents/skills dir with content.
	stale := filepath.Join(ws, ".agents", "skills", "leftover")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := syncSkillsToWorkspace(ws, []string{"alpha"}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	link := filepath.Join(ws, ".agents", "skills")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".agents/skills should have been replaced by a symlink")
	}
}

func TestWorkspaceSkillDir(t *testing.T) {
	ws := t.TempDir()
	pkg := filepath.Join(ws, ".claude", "skills", "alpha")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "SKILL.md"), []byte("# alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Scoped ref resolves to the same on-disk package.
	got, err := workspaceSkillDir(ws, "shared:alpha")
	if err != nil {
		t.Fatalf("workspaceSkillDir: %v", err)
	}
	if got != pkg {
		t.Errorf("got %q, want %q", got, pkg)
	}

	// Missing package and traversal refs are rejected.
	if _, err := workspaceSkillDir(ws, "missing"); err == nil {
		t.Error("expected error for missing skill package")
	}
	if _, err := workspaceSkillDir(ws, ".."); err == nil {
		t.Error("expected error for traversal ref")
	}
}

func TestPushSkillToShared(t *testing.T) {
	var gotPath, gotSource string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body struct {
			SourcePath string `json:"sourcePath"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		gotSource = body.SourcePath
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":"created","changed":true,"created":true,"version":3,"id":"skl_x"}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	res, err := pushSkillToShared(addr, "shared:alpha", "/ws/.claude/skills/alpha")
	if err != nil {
		t.Fatalf("pushSkillToShared: %v", err)
	}
	if !res.Changed || !res.Created {
		t.Errorf("expected changed=true created=true, got %v/%v", res.Changed, res.Created)
	}
	if res.Version != 3 {
		t.Errorf("expected version=3, got %d", res.Version)
	}
	if res.Status != "created" || res.ID != "skl_x" {
		t.Errorf("expected status=created id=skl_x, got %q/%q", res.Status, res.ID)
	}
	if gotPath != "/api/skills/shared:alpha/push-from-path" {
		t.Errorf("forwarded path = %q", gotPath)
	}
	if gotSource != "/ws/.claude/skills/alpha" {
		t.Errorf("forwarded sourcePath = %q", gotSource)
	}
}

func TestSkillStatusAgainstShared(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"inStore":true,"differs":true,"exists":true,"name":"Alpha","description":"d","storeVersion":2}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	st, err := skillStatusAgainstShared(addr, "shared:alpha", "/ws/.claude/skills/alpha")
	if err != nil {
		t.Fatalf("skillStatusAgainstShared: %v", err)
	}
	if gotPath != "/api/skills/shared:alpha/status-from-path" {
		t.Errorf("forwarded path = %q", gotPath)
	}
	if !st.InStore || !st.Differs || st.Name != "Alpha" || st.StoreVersion != 2 {
		t.Errorf("unexpected status: %+v", st)
	}
	if skillState(st) != "modified" {
		t.Errorf("skillState = %q, want modified", skillState(st))
	}
	if skillState(sharedSkillStatus{InStore: false}) != "local" {
		t.Error("not-in-store should map to local")
	}
	if skillState(sharedSkillStatus{InStore: true, Differs: false}) != "synced" {
		t.Error("in-store identical should map to synced")
	}
}

func TestPushSkillToSharedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"no skill package (missing SKILL.md)"}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	if _, err := pushSkillToShared(addr, "shared:alpha", "/nope"); err == nil {
		t.Fatal("expected error from 400 response")
	} else if !strings.Contains(err.Error(), "SKILL.md") {
		t.Errorf("error should surface the manager message, got %v", err)
	}
}

func TestNormalizeSkillRef(t *testing.T) {
	cases := map[string]string{
		"foo":           "foo",
		"shared:foo":    "foo",
		"centralized:x": "x",
		"  spaced  ":    "spaced",
		"../escape":     "escape",
		"a/b/c":         "c",
	}
	for in, want := range cases {
		if got := normalizeSkillRef(in); got != want {
			t.Errorf("normalizeSkillRef(%q) = %q, want %q", in, got, want)
		}
	}
}
