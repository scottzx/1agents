package main

import (
	"strings"
	"testing"
)

func TestParseSubject(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantOK  bool
		wantTyp string
		wantSc  string
		wantSum string
		wantPR  int
	}{
		{
			name:    "squash feat with nested PR markers",
			in:      "feat(task-engine): 执行结果即提案 — 对抗式多校验者验证 (#131) (#260)",
			wantOK:  true,
			wantTyp: "feat",
			wantSc:  "task-engine",
			wantSum: "执行结果即提案 — 对抗式多校验者验证",
			wantPR:  260,
		},
		{
			name:    "fix without scope",
			in:      "fix: 修复登录崩溃 (#42)",
			wantOK:  true,
			wantTyp: "fix",
			wantSc:  "",
			wantSum: "修复登录崩溃",
			wantPR:  42,
		},
		{
			name:    "breaking change marker",
			in:      "feat(api)!: 重写鉴权 (#7)",
			wantOK:  true,
			wantTyp: "feat",
			wantSc:  "api",
			wantSum: "重写鉴权",
			wantPR:  7,
		},
		{
			name:   "non-conventional line skipped",
			in:     "Merge branch 'main' into feature",
			wantOK: false,
		},
		{
			name:    "no PR marker",
			in:      "docs: 更新 README",
			wantOK:  true,
			wantTyp: "docs",
			wantSum: "更新 README",
			wantPR:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := ParseSubject(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if f.Type != tc.wantTyp {
				t.Errorf("Type = %q, want %q", f.Type, tc.wantTyp)
			}
			if f.Scope != tc.wantSc {
				t.Errorf("Scope = %q, want %q", f.Scope, tc.wantSc)
			}
			if f.Summary != tc.wantSum {
				t.Errorf("Summary = %q, want %q", f.Summary, tc.wantSum)
			}
			if f.PR != tc.wantPR {
				t.Errorf("PR = %d, want %d", f.PR, tc.wantPR)
			}
		})
	}
}

func TestGroupFeaturesOrdering(t *testing.T) {
	feats := ParseLog([]string{
		"chore: 杂事 (#1)",
		"feat(a): 功能一 (#2)",
		"fix: 修复 (#3)",
		"feat(b): 功能二 (#4)",
		"weird(x): 自定义类型 (#5)",
	})
	groups := GroupFeatures(feats)

	// feat first, then fix, then chore, then unknown "weird".
	wantOrder := []string{"feat", "fix", "chore", "weird"}
	if len(groups) != len(wantOrder) {
		t.Fatalf("got %d groups, want %d", len(groups), len(wantOrder))
	}
	for i, w := range wantOrder {
		if groups[i].Type != w {
			t.Errorf("group[%d] = %q, want %q", i, groups[i].Type, w)
		}
	}
	if len(groups[0].Features) != 2 {
		t.Errorf("feat group should have 2 features, got %d", len(groups[0].Features))
	}
}

func TestRenderContainsFeaturesAndLinks(t *testing.T) {
	feats := ParseLog([]string{
		"feat(task-engine): 执行结果即提案 (#131) (#260)",
		"fix: 修复登录崩溃 (#42)",
	})
	data := PageData{
		Version:    "v20260625-1",
		PrevRef:    "v20260624-1",
		CommitTo:   "HEAD",
		Repo:       "scottzx/1agents",
		Generated:  "2026-06-25 00:00 UTC",
		Groups:     GroupFeatures(feats),
		TotalCount: len(feats),
	}
	html, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	mustContain := []string{
		"v20260625-1",
		"执行结果即提案",
		"修复登录崩溃",
		"task-engine",
		"新功能",
		"问题修复",
		`https://github.com/scottzx/1agents/pull/260`,
		`https://github.com/scottzx/1agents/pull/42`,
		"<!DOCTYPE html>",
		"v20260624-1..HEAD",
	}
	for _, s := range mustContain {
		if !strings.Contains(html, s) {
			t.Errorf("rendered HTML missing %q", s)
		}
	}
}

func TestRenderWithoutRepoFallsBackToPlainPR(t *testing.T) {
	feats := ParseLog([]string{"feat: 离线功能 (#99)"})
	data := PageData{
		Version:    "dev",
		CommitTo:   "HEAD",
		Repo:       "", // offline / no remote
		Generated:  "now",
		Groups:     GroupFeatures(feats),
		TotalCount: len(feats),
	}
	html, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(html, "github.com") {
		t.Errorf("expected no PR link when repo is empty, but found github.com")
	}
	if !strings.Contains(html, "#99") {
		t.Errorf("expected PR number as plain text")
	}
}

// fakeGit lets us exercise range/version resolution without a real repo.
type fakeGit struct{ responses map[string]string }

func (f fakeGit) run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	if v, ok := f.responses[key]; ok {
		return v, nil
	}
	return "", errNotFound
}

var errNotFound = &gitError{}

type gitError struct{}

func (*gitError) Error() string { return "not found" }

func TestDetectRepoFromRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:scottzx/1agents.git":     "scottzx/1agents",
		"https://github.com/scottzx/1agents.git": "scottzx/1agents",
		"https://github.com/scottzx/1agents":     "scottzx/1agents",
	}
	for url, want := range cases {
		g := fakeGit{responses: map[string]string{"config --get remote.origin.url": url}}
		if got := detectRepo(g); got != want {
			t.Errorf("detectRepo(%q) = %q, want %q", url, got, want)
		}
	}
	// No remote -> empty.
	if got := detectRepo(fakeGit{responses: map[string]string{}}); got != "" {
		t.Errorf("detectRepo with no remote = %q, want empty", got)
	}
}

func TestCollectSubjectsRange(t *testing.T) {
	g := fakeGit{responses: map[string]string{
		"log --no-merges --pretty=format:%s v1..HEAD": "feat: a (#1)\nfix: b (#2)",
	}}
	got, err := collectSubjects(g, "v1", "HEAD")
	if err != nil {
		t.Fatalf("collectSubjects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d subjects, want 2", len(got))
	}
}
