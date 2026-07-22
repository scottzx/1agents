package meta

import (
	"fmt"
	"strings"
)

// ExecutorAssignment is the normalized executor×assignee pair (名称定义表 §0.5).
// FunctionType is only meaningful when Executor=function; on write it is the
// source of truth and Assignee is mirrored to the same value.
type ExecutorAssignment struct {
	Executor     TaskExecutor
	Assignee     string
	FunctionType string
}

// NormalizeExecutorAssignment validates the executor×assignee matrix and returns
// a filled assignment. Single source of truth for taskapi Dispatch and HTTP
// project-items create/patch (#192 / #198).
//
// Rules:
//
//	executor empty + assignee=user → human (board create convenience)
//	executor empty otherwise       → agent
//	executor=agent                 → assignee empty or AgentType (never "user")
//	executor=human                 → assignee forced to "user"
//	executor=function              → FunctionType required (or assignee as name);
//	                                 Assignee mirrored to FunctionType
//
// Field name stays executor (not AIWorkforce). Invalid combos return a
// descriptive error suitable for HTTP 4xx.
func NormalizeExecutorAssignment(executor TaskExecutor, assignee, functionType string) (ExecutorAssignment, error) {
	out := ExecutorAssignment{
		Executor:     executor,
		Assignee:     strings.TrimSpace(assignee),
		FunctionType: strings.TrimSpace(functionType),
	}

	if out.Executor == "" {
		if out.Assignee == AssigneeUser {
			out.Executor = TaskExecutorHuman
		} else {
			out.Executor = TaskExecutorAgent
		}
	}

	switch out.Executor {
	case TaskExecutorAgent:
		if out.Assignee == AssigneeUser {
			return out, fmt.Errorf("executor=agent cannot use assignee=user (use executor=human)")
		}
	case TaskExecutorHuman:
		out.Assignee = AssigneeUser
		out.FunctionType = ""
	case TaskExecutorFunction:
		fn := out.FunctionType
		if fn == "" {
			fn = out.Assignee
		}
		if fn == "" {
			return out, fmt.Errorf("executor=function requires FunctionType (or assignee=function name)")
		}
		out.FunctionType = fn
		out.Assignee = fn
	default:
		return out, fmt.Errorf("invalid executor %q (want agent|function|human)", out.Executor)
	}
	return out, nil
}

// ApplyFnLabel ensures Labels contain exactly one fn:<type> entry for function
// tasks (and none otherwise). Returns a new slice.
func ApplyFnLabel(labels []string, exec TaskExecutor, functionType string) []string {
	out := make([]string, 0, len(labels)+1)
	for _, l := range labels {
		if strings.HasPrefix(l, "fn:") {
			continue
		}
		out = append(out, l)
	}
	if exec == TaskExecutorFunction && functionType != "" {
		out = append(out, "fn:"+functionType)
	}
	return out
}
