package meta

import "testing"

func TestNormalizeExecutorAssignment(t *testing.T) {
	a, err := NormalizeExecutorAssignment("", "", "")
	if err != nil || a.Executor != TaskExecutorAgent || a.Assignee != "" {
		t.Fatalf("default agent: %+v err=%v", a, err)
	}
	h, err := NormalizeExecutorAssignment("", AssigneeUser, "")
	if err != nil || h.Executor != TaskExecutorHuman || h.Assignee != AssigneeUser {
		t.Fatalf("user→human: %+v err=%v", h, err)
	}
	if _, err := NormalizeExecutorAssignment(TaskExecutorAgent, AssigneeUser, ""); err == nil {
		t.Fatal("expected agent+user error")
	}
	h2, err := NormalizeExecutorAssignment(TaskExecutorHuman, "anything", "")
	if err != nil || h2.Assignee != AssigneeUser {
		t.Fatalf("human force user: %+v err=%v", h2, err)
	}
	f, err := NormalizeExecutorAssignment(TaskExecutorFunction, "", "core.noop")
	if err != nil || f.Assignee != "core.noop" || f.FunctionType != "core.noop" {
		t.Fatalf("function type: %+v err=%v", f, err)
	}
	f2, err := NormalizeExecutorAssignment(TaskExecutorFunction, "core.noop", "")
	if err != nil || f2.FunctionType != "core.noop" {
		t.Fatalf("function assignee: %+v err=%v", f2, err)
	}
	if _, err := NormalizeExecutorAssignment(TaskExecutorFunction, "", ""); err == nil {
		t.Fatal("expected function without type error")
	}
	if _, err := NormalizeExecutorAssignment("AIWorkforce", "", ""); err == nil {
		t.Fatal("expected invalid executor error")
	}
}

func TestApplyFnLabel(t *testing.T) {
	in := []string{"a", "fn:old", "b"}
	out := ApplyFnLabel(in, TaskExecutorFunction, "core.noop")
	if len(out) != 3 || out[2] != "fn:core.noop" {
		t.Fatalf("function labels: %v", out)
	}
	out2 := ApplyFnLabel(in, TaskExecutorAgent, "")
	if len(out2) != 2 || out2[0] != "a" || out2[1] != "b" {
		t.Fatalf("agent strips fn: %v", out2)
	}
}

func TestProjectItemIsPrimaryType(t *testing.T) {
	var p ProjectItem
	p.Type = ItemTypeTask
	var task Task = p
	if task.Type != ItemTypeTask {
		t.Fatal("Task alias lost Type")
	}
	var back ProjectItem = task
	if back.Type != ItemTypeTask {
		t.Fatal("ProjectItem round-trip failed")
	}
}
