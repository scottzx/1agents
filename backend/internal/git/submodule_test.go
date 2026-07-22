package git

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSubmoduleStatus(t *testing.T) {
	out := " 244a18e8944cf01cb81ea3f5178ae10f4e881233 modules/1acp (v0.12.0-52-g244a18e)\n" +
		"+bc1a6703399f93b1bc3fd4936f38b805ab21338d modules/cc-switch-cli (v5.9.1)\n" +
		"-80ea6ea18b8937db0d453e3f5440fdfbc4778a32 modules/example\n" +
		"U979ba4be341534731445e4b11a84f0c0aa4b40a8 modules/cc-connect (v1)\n"
	got := parseSubmoduleStatus(out)
	if len(got) != 4 {
		t.Fatalf("len=%d want 4: %+v", len(got), got)
	}
	if got[0].Flag != "" || got[0].Path != "modules/1acp" || got[0].Short != "244a18e" {
		t.Fatalf("entry0: %+v", got[0])
	}
	if got[1].Flag != "+" || got[1].Path != "modules/cc-switch-cli" {
		t.Fatalf("entry1: %+v", got[1])
	}
	if got[2].Flag != "-" || got[2].Desc != "" {
		t.Fatalf("entry2: %+v", got[2])
	}
	if got[3].Flag != "U" {
		t.Fatalf("entry3: %+v", got[3])
	}
}

func TestSubmoduleBranchAndCommitWhenParentMarksUninitialized(t *testing.T) {
	root, modulePath := setupDetachedSubmodule(t)
	handler := NewHandler(root)

	recorder := httptest.NewRecorder()
	handler.Submodules(recorder, httptest.NewRequest(http.MethodGet, "/api/git/submodules", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("submodules status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var entries []SubmoduleEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Flag != "-" || entries[0].Branch != "main" {
		t.Fatalf("entries=%+v", entries)
	}

	if err := os.WriteFile(filepath.Join(modulePath, "new.txt"), []byte("submodule change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	requestPath := url.QueryEscape("modules/child")
	assertHandlerOK(t, handler.Stage, http.MethodPost, "/api/git/stage?file=new.txt&path="+requestPath, nil)
	assertHandlerOK(t, handler.Commit, http.MethodPost, "/api/git/commit?path="+requestPath, []byte(`{"message":"submodule commit"}`))
	if got := runGit(t, modulePath, "log", "-1", "--pretty=%s"); got != "submodule commit" {
		t.Fatalf("submodule commit=%q", got)
	}

	recorder = httptest.NewRecorder()
	handler.Graph(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/git/graph?limit=10&path="+requestPath, nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("graph status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var commits []GraphCommit
	if err := json.Unmarshal(recorder.Body.Bytes(), &commits); err != nil {
		t.Fatal(err)
	}
	if len(commits) == 0 || commits[0].Message != "submodule commit" {
		t.Fatalf("submodule graph=%+v", commits)
	}

	head := runGit(t, modulePath, "rev-parse", "HEAD")
	recorder = httptest.NewRecorder()
	handler.CommitFiles(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/git/commit-files?hash="+head+"&path="+requestPath, nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("commit files status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var files []CommitFileEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "new.txt" {
		t.Fatalf("submodule commit files=%+v", files)
	}
}

func TestWorktreeStageAndCommitUseSelectedPath(t *testing.T) {
	root := initRepo(t, filepath.Join(t.TempDir(), "root"))
	worktree := filepath.Join(t.TempDir(), "worktree")
	runGit(t, root, "worktree", "add", "-b", "feature", worktree)
	if err := os.WriteFile(filepath.Join(worktree, "feature.txt"), []byte("worktree change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(root)
	requestPath := url.QueryEscape(worktree)
	assertHandlerOK(t, handler.Stage, http.MethodPost, "/api/git/stage?file=feature.txt&path="+requestPath, nil)
	assertHandlerOK(t, handler.Commit, http.MethodPost, "/api/git/commit?path="+requestPath, []byte(`{"message":"worktree commit"}`))
	if got := runGit(t, worktree, "log", "-1", "--pretty=%s"); got != "worktree commit" {
		t.Fatalf("worktree commit=%q", got)
	}
	if got := runGit(t, root, "log", "-1", "--pretty=%s"); got != "initial" {
		t.Fatalf("main commit changed: %q", got)
	}
}

func setupDetachedSubmodule(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	root := initRepo(t, filepath.Join(base, "root"))
	child := initRepo(t, filepath.Join(base, "child"))
	runGit(t, root, "-c", "protocol.file.allow=always", "submodule", "add", child, "modules/child")
	runGit(t, root, "commit", "-am", "add submodule")
	runGit(t, root, "submodule", "deinit", "-f", "modules/child")
	modulePath := filepath.Join(root, "modules", "child")
	runGit(t, base, "clone", child, modulePath)
	return root, modulePath
}

func initRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func assertHandlerOK(
	t *testing.T,
	handler func(http.ResponseWriter, *http.Request),
	method string,
	target string,
	body []byte,
) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(method, target, bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s status=%d body=%s", method, target, recorder.Code, recorder.Body.String())
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
