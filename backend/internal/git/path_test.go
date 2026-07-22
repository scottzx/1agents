package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestUnquoteGitPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`plain.txt`, `plain.txt`},
		{`"file with spaces.txt"`, `file with spaces.txt`},
		// UTF-8 for 更新项目.txt as C-style octal (git default quotepath)
		{`"\346\233\264\346\226\260\351\241\271\347\233\256.txt"`, "更新项目.txt"},
		{`"foo\"bar"`, `foo"bar`},
		{`  "\346\233\264.txt"  `, "更.txt"},
	}
	for _, tc := range cases {
		got := unquoteGitPath(tc.in)
		if got != tc.want {
			t.Errorf("unquoteGitPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestChangedFilesChinesePath(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	// Force quotepath on so the regression is real without relying on global config.
	run("config", "core.quotepath", "true")

	name := "更新项目.txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", name)

	h := NewHandler(dir)
	staged, unstaged, untracked := h.changedFilesAt(dir)
	if len(staged) != 1 || staged[0].Path != name {
		t.Fatalf("staged=%+v unstaged=%+v untracked=%+v, want path %q", staged, unstaged, untracked, name)
	}
	// Confirm we did not leave C-escapes in the path.
	if staged[0].Path == "" || staged[0].Path[0] == '"' || staged[0].Path[0] == '\\' {
		t.Fatalf("path still escaped: %q", staged[0].Path)
	}
}
