// Package taskapi is the North Task API: the clean boundary that applications
// call to dispatch, query, and receive callbacks for tasks. Applications NEVER
// embed agents or import the runner directly — they go through this API only.
//
// Design (#320):
//   - DispatchTask creates a task with executor/business_ref/target and returns its id.
//   - QueryTask / QueryTasks return live task state.
//   - RegisterCompletionHook registers an application callback invoked when a
//     task reaches a terminal state (completed/failed/cancelled).
//   - Per-application permission is enforced via a simple namespace allowlist:
//     an app may only dispatch task types it declared at startup.
//
// Wave 2/3 apps call this package; the scheduler/runner/function-runner are
// internal plumbing that apps must not touch.
package taskapi

import (
	"fmt"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// DispatchSpec is the payload an application passes to DispatchTask.
type DispatchSpec struct {
	// Title is the task's short human-readable label.
	Title string
	// Description is the work instruction (Markdown). For agent tasks this is
	// sent as the prompt; for function tasks it is informational only.
	Description string
	// AcceptanceCriteria is injected into agent prompts as the self-check gate.
	AcceptanceCriteria string
	// Executor selects the execution path. Defaults to "agent".
	// Values: agent | function | human (field name stays executor — not AIWorkforce).
	Executor meta.TaskExecutor
	// Assignee is the channel object (名称定义表 §0.5):
	//   agent    → AgentType (empty = runner default)
	//   human    → "user" (forced on write)
	//   function → function name; mirrored from FunctionType when empty
	Assignee string
	// FunctionType is the registered handler key for executor=function tasks
	// (e.g. "core.noop"). On write it is the source of truth and Assignee is
	// mirrored to the same value (#192). Ignored for agent/human.
	FunctionType string
	// BusinessRef is the opaque binding seam, e.g. "crm:lead:42". Nullable.
	BusinessRef string
	// Target overrides dispatch defaults (agent type, cwd, capabilities).
	Target *meta.TaskTargetSpec
	// DependsOn is the list of task IDs this task must wait for.
	DependsOn []string
	// Priority is "urgent"|"high"|"medium"|"low". Defaults to "medium".
	Priority string
	// Milestone is the roadmap stage label.
	Milestone string
	// WorkspacePath is the project directory. Required.
	WorkspacePath string
}

// CompletionEvent is the payload delivered to a CompletionHook.
type CompletionEvent struct {
	TaskID      string
	Status      meta.TaskStatus // completed | failed | cancelled
	Result      string          // JSON result written by the executor
	CostTokens  int64
	CompletedAt time.Time
}

// CompletionHook is a callback registered by an application. It is called
// synchronously from the post-finish path; keep it fast (hand off to a goroutine
// for heavy work).
type CompletionHook func(ev CompletionEvent)

// AppPermissions declares which task types an application is allowed to dispatch.
// The namespace is the app's identifier (e.g. "crm", "media", "radio"). Types
// are free-form strings; an empty list means "any type allowed" (for the kernel
// itself). Phase 1 single-user: this is an honour-based allowlist, not enforced
// cryptographically.
type AppPermissions struct {
	Namespace    string
	AllowedTypes []string // empty = unrestricted
	AllowedRefs  []string // business_ref prefixes allowed; empty = any
}

// API is the North Task API service. Construct it with New and wire it into
// the server at startup.
type API struct {
	store *meta.TaskStore

	mu    sync.RWMutex
	perms map[string]*AppPermissions // namespace → permissions
	hooks []CompletionHook
}

// New returns an API over store.
func New(store *meta.TaskStore) *API {
	return &API{
		store: store,
		perms: make(map[string]*AppPermissions),
	}
}

// RegisterApp declares an application's permissions. Must be called at startup
// before any DispatchTask call from that namespace. Idempotent for the same
// namespace (last write wins).
func (a *API) RegisterApp(p AppPermissions) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.perms[p.Namespace] = &p
}

// RegisterCompletionHook adds a callback invoked when any task reaches a
// terminal state. Wave 3 apps register their writeback handlers here. The hook
// is called from the finish path; avoid blocking.
func (a *API) RegisterCompletionHook(h CompletionHook) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hooks = append(a.hooks, h)
}

// NotifyCompletion is called by the runner / function-runner / human-complete
// path when a task reaches a terminal state. It fires all registered hooks.
func (a *API) NotifyCompletion(ev CompletionEvent) {
	a.mu.RLock()
	hooks := make([]CompletionHook, len(a.hooks))
	copy(hooks, a.hooks)
	a.mu.RUnlock()
	for _, h := range hooks {
		h(ev)
	}
}

// checkPermission validates that namespace is allowed to dispatch a task with
// the given businessRef. Returns nil when allowed.
func (a *API) checkPermission(namespace, businessRef string) error {
	if namespace == "" {
		return nil // kernel / unregistered callers are unrestricted in Phase 1
	}
	a.mu.RLock()
	p, ok := a.perms[namespace]
	a.mu.RUnlock()
	if !ok {
		return nil // unregistered namespace: unrestricted in Phase 1 single-user
	}
	if len(p.AllowedRefs) == 0 {
		return nil
	}
	for _, prefix := range p.AllowedRefs {
		if len(businessRef) >= len(prefix) && businessRef[:len(prefix)] == prefix {
			return nil
		}
	}
	return fmt.Errorf("taskapi: namespace %q not allowed to dispatch ref %q", namespace, businessRef)
}

// DispatchTask creates a task and enqueues it for execution. namespace is the
// calling application's identifier (empty = kernel). Returns the new task ID.
//
// Executor × assignee matrix (名称定义表 §0.5 / #192):
//
//	executor=agent    → assignee empty or AgentType (never "user")
//	executor=human    → assignee fixed to "user"
//	executor=function → FunctionType required; Assignee mirrored to FunctionType;
//	                    Labels also carry "fn:<FunctionType>" for the runner
//
// Field name stays executor (not AIWorkforce). function_type is the DispatchSpec
// field; on write it is the source of truth and assignee is mirrored to the same value.
func (a *API) DispatchTask(namespace string, spec DispatchSpec) (string, error) {
	if err := a.checkPermission(namespace, spec.BusinessRef); err != nil {
		return "", err
	}
	if spec.WorkspacePath == "" {
		return "", fmt.Errorf("taskapi: WorkspacePath is required")
	}

	normalized, err := NormalizeDispatchSpec(spec)
	if err != nil {
		return "", err
	}
	spec = normalized

	now := time.Now().UTC()
	taskID := meta.NewID()
	t := meta.Task{
		ID:                 taskID, // pre-assign so Mutate captures it immediately
		Title:              spec.Title,
		Description:        spec.Description,
		AcceptanceCriteria: spec.AcceptanceCriteria,
		Executor:           spec.Executor,
		Assignee:           spec.Assignee,
		BusinessRef:        spec.BusinessRef,
		TaskTarget:         spec.Target,
		DependsOn:          spec.DependsOn,
		Priority:           meta.Priority(priorityOrDefault(spec.Priority)),
		Milestone:          spec.Milestone,
		Status:             meta.TaskStatusPending,
		IssueState:         meta.IssueOpen,
		Type:               meta.ItemTypeTask,
		CreatedBy:          namespace,
		CreatedAt:          now,
		UpdatedAt:          now,
		MaxRetries:         1,
		Replies:            []meta.Reply{},
		Sessions:           []meta.SessionMetadata{},
	}
	// For function tasks, embed the handler type in Labels so the runner can
	// look it up without a dedicated column. Mirrors Assignee (same value).
	t.Labels = meta.ApplyFnLabel(t.Labels, spec.Executor, spec.FunctionType)

	err = a.store.Mutate(spec.WorkspacePath, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, t)
		return true
	})
	if err != nil {
		return "", fmt.Errorf("taskapi: dispatch: %w", err)
	}
	return taskID, nil
}

// NormalizeDispatchSpec validates the executor×assignee matrix and returns a
// copy with Executor defaulted and Assignee filled. Delegates to
// meta.NormalizeExecutorAssignment — the single matrix entry used by HTTP
// project-items create/patch as well (#192 / #198 / 名称定义表 §0.5).
func NormalizeDispatchSpec(spec DispatchSpec) (DispatchSpec, error) {
	asg, err := meta.NormalizeExecutorAssignment(spec.Executor, spec.Assignee, spec.FunctionType)
	if err != nil {
		return spec, fmt.Errorf("taskapi: %w", err)
	}
	out := spec
	out.Executor = asg.Executor
	out.Assignee = asg.Assignee
	out.FunctionType = asg.FunctionType
	return out, nil
}

// QueryTask returns the current state of task by id. ok=false when not found.
func (a *API) QueryTask(id string) (meta.Task, bool, error) {
	return a.store.GetTask(id)
}

// QueryTasks returns tasks for a workspace, optionally filtered. Pass empty
// strings to skip a filter.
func (a *API) QueryTasks(workspacePath, businessRef, executorFilter string) ([]meta.Task, error) {
	cfg, err := a.store.Load(workspacePath)
	if err != nil {
		return nil, err
	}
	if businessRef == "" && executorFilter == "" {
		return cfg.Tasks, nil
	}
	var out []meta.Task
	for _, t := range cfg.Tasks {
		if businessRef != "" && t.BusinessRef != businessRef {
			continue
		}
		if executorFilter != "" && string(t.Executor) != executorFilter {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// helpers ──────────────────────────────────────────────────────────────────


func priorityOrDefault(p string) string {
	if p == "" {
		return "medium"
	}
	return p
}
