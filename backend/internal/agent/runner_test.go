package agent

import (
	"strings"
	"testing"
)

func TestBuildTaskInstructionAppendsProjectExecutorPrompt(t *testing.T) {
	got := buildTaskInstruction(Task{
		Title:              "样式 token",
		Description:        "实现语义 token",
		AcceptanceCriteria: "通过自查",
	})

	if !strings.HasPrefix(got, "实现语义 token") {
		t.Fatalf("instruction prefix = %q", got)
	}
	if strings.Contains(got, "/PM") {
		t.Fatalf("instruction should not trigger PM skill: %q", got)
	}
	if !strings.Contains(got, "=== 验收标准 ===\n通过自查") {
		t.Fatalf("instruction missing acceptance criteria: %q", got)
	}
	if !strings.HasSuffix(got, projectExecutorPrompt) {
		t.Fatalf("instruction missing project_executor prompt suffix: %q", got)
	}
	if !strings.Contains(got, "1agents project-items get <任务ID>") {
		t.Fatalf("project_executor prompt should teach the built-in CLI: %q", got)
	}
}

func TestBuildTaskInstructionUsesFrontmatterAcceptance(t *testing.T) {
	got := buildTaskInstruction(Task{
		Description:        "---\nacceptance: frontmatter criteria\n---\n正文",
		AcceptanceCriteria: "legacy criteria",
	})

	if !strings.Contains(got, "=== 验收标准 ===\nfrontmatter criteria") {
		t.Fatalf("instruction should prefer frontmatter acceptance: %q", got)
	}
	if strings.Contains(got, "legacy criteria") {
		t.Fatalf("instruction should not include legacy acceptance when frontmatter wins: %q", got)
	}
}

func TestBuildTaskInstructionFallsBackToTitle(t *testing.T) {
	got := buildTaskInstruction(Task{Title: "只有标题"})
	want := "只有标题\n\n" + projectExecutorPrompt
	if got != want {
		t.Fatalf("instruction = %q, want %q", got, want)
	}
}
