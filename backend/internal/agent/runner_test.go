package agent

import (
	"strings"
	"testing"
)

func TestBuildTaskInstructionAppendsProjectExecutorPrompt(t *testing.T) {
	got := buildTaskInstruction(Task{
		ID:                 "task-uuid-1",
		Number:             25,
		Title:              "样式 token",
		Description:        "实现语义 token",
		AcceptanceCriteria: "通过自查",
	}, "ws-1", "/tmp/ws-one")

	if !strings.HasPrefix(got, "实现语义 token") {
		t.Fatalf("instruction prefix = %q", got)
	}
	if strings.Contains(got, "/PM") {
		t.Fatalf("instruction should not trigger PM skill: %q", got)
	}
	if !strings.Contains(got, "=== 验收标准 ===\n通过自查") {
		t.Fatalf("instruction missing acceptance criteria: %q", got)
	}
	if !strings.Contains(got, "=== project_executor ===") {
		t.Fatalf("instruction missing project_executor prompt suffix: %q", got)
	}
	for _, want := range []string{
		"Task: #25 / task-uuid-1",
		"Workspace ID: ws-1",
		"Workspace Path: /tmp/ws-one",
		"ONEAGENTS_WORKSPACE_ID=ws-1",
		" get task-uuid-1 --json",
		"短编号必须带 #,例如 #25;裸数字 25 会被当成 UUID",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("project_executor prompt missing %q: %q", want, got)
		}
	}
}

func TestBuildTaskInstructionUsesFrontmatterAcceptance(t *testing.T) {
	got := buildTaskInstruction(Task{
		Description:        "---\nacceptance: frontmatter criteria\n---\n正文",
		AcceptanceCriteria: "legacy criteria",
	}, "ws-1", "/tmp/ws-one")

	if !strings.Contains(got, "=== 验收标准 ===\nfrontmatter criteria") {
		t.Fatalf("instruction should prefer frontmatter acceptance: %q", got)
	}
	if strings.Contains(got, "legacy criteria") {
		t.Fatalf("instruction should not include legacy acceptance when frontmatter wins: %q", got)
	}
}

func TestBuildTaskInstructionFallsBackToTitle(t *testing.T) {
	got := buildTaskInstruction(Task{ID: "task-uuid-2", Title: "只有标题"}, "ws-2", "/tmp/ws-two")
	if !strings.HasPrefix(got, "只有标题\n\n=== project_executor ===") {
		t.Fatalf("instruction = %q", got)
	}
	if !strings.Contains(got, "Task UUID: task-uuid-2") {
		t.Fatalf("instruction missing task uuid: %q", got)
	}
}

func TestBuildTaskInstructionAppendsFunctionContext(t *testing.T) {
	got := buildTaskInstructionWithPreamble(Task{
		ID:          "task-uuid-4",
		Title:       "摘要邮件",
		Description: "根据 function_context 写摘要",
	}, "ws-4", "/tmp/ws-four", `{"unread":2,"subjects":["a","b"]}`)

	if !strings.Contains(got, "=== function_context ===") || !strings.Contains(got, `"unread": 2`) {
		t.Fatalf("missing pretty function_context: %q", got)
	}
	if !strings.Contains(got, "=== end function_context ===") {
		t.Fatalf("missing end marker: %q", got)
	}
	if !strings.Contains(got, "不要改写其中的原始字段") {
		t.Fatalf("missing fact-preservation line: %q", got)
	}
	if !strings.Contains(got, "=== project_executor ===") {
		t.Fatalf("project_executor should still be appended: %q", got)
	}
	plain := buildTaskInstruction(Task{ID: "task-uuid-4", Title: "摘要邮件", Description: "根据 function_context 写摘要"}, "ws-4", "/tmp/ws-four")
	if strings.Contains(plain, "=== function_context ===") {
		t.Fatalf("no-preamble instruction should stay unchanged: %q", plain)
	}
}

func TestProjectItemsCLIHonorsNpmShimEnv(t *testing.T) {
	t.Setenv("ONEAGENTS_CLI", "/usr/local/bin/1agents")

	got := buildProjectExecutorPrompt(Task{ID: "task-uuid-3"}, "ws-3", "/tmp/ws-three")
	if !strings.Contains(got, "/usr/local/bin/1agents project-items get task-uuid-3 --json") {
		t.Fatalf("instruction should use ONEAGENTS_CLI npm shim: %q", got)
	}
}
