package taskapi

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// FunctionContext is passed to a FunctionHandler. It carries the task details
// and provides helpers to write results back.
type FunctionContext struct {
	Task meta.Task
	// CostTokens should be left 0 for pure in-process functions; set it when
	// calling an external LLM/service that incurs a token cost.
	CostTokens int64
}

// FunctionHandler is the interface Wave 3 apps implement for each function type.
// The handler must return a JSON-marshallable result or an error. On success,
// the runner writes the result to task.result and marks the task completed. On
// error, the task is failed (retry budget applies as normal).
type FunctionHandler func(ctx FunctionContext) (result any, err error)

// Registry holds the map of type → handler. There is one global registry
// (globalFunctions) and apps register into it via RegisterFunction.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]FunctionHandler
}

var globalFunctions = &Registry{handlers: make(map[string]FunctionHandler)}

// RegisterFunction registers handler under the given type key. The key is used
// in task Labels as "fn:<type>" (e.g. "fn:core.noop", or an app-registered type).
// One registration, two consumptions: the function runner picks it up as a
// standalone task; agent tools call it via the MCP function-call path (Wave 3).
// Safe for concurrent use; last registration for a key wins.
func RegisterFunction(typeName string, handler FunctionHandler) {
	globalFunctions.mu.Lock()
	defer globalFunctions.mu.Unlock()
	globalFunctions.handlers[typeName] = handler
}

// Lookup returns the handler for typeName, or nil when unregistered.
func Lookup(typeName string) FunctionHandler {
	globalFunctions.mu.RLock()
	defer globalFunctions.mu.RUnlock()
	return globalFunctions.handlers[typeName]
}

// ExtractFunctionType extracts the "fn:<type>" label from a task's Labels slice.
// Returns "" when no fn: label is present.
func ExtractFunctionType(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "fn:") {
			return strings.TrimPrefix(l, "fn:")
		}
	}
	return ""
}

// RunFunction executes a function task synchronously (in the calling goroutine).
// It should be invoked from the scheduler's ready loop when executor==function,
// analogous to how runner.Execute is called for agent tasks. On return, the task
// is in a terminal state (completed / failed).
//
// The store parameter is used to write back result + status; the api parameter
// fires completion hooks. Both may be nil for unit tests.
func RunFunction(task meta.Task, workspacePath string, store *meta.TaskStore, api *API) {
	runFunction(task, workspacePath, store, api, meta.TaskRun{})
}

// RunFunctionWithRunMetadata is the FunctionExecutor entry point for an
// ExecutionJob. It keeps function execution on the shared TaskRun audit spine.
func RunFunctionWithRunMetadata(task meta.Task, workspacePath string, store *meta.TaskStore, api *API, runMetadata meta.TaskRun) {
	runFunction(task, workspacePath, store, api, runMetadata)
}

func runFunction(task meta.Task, workspacePath string, store *meta.TaskStore, api *API, runMetadata meta.TaskRun) {
	// Prefer Labels "fn:<type>"; fall back to Assignee (mirrored on write, #192).
	fnType := ExtractFunctionType(task.Labels)
	if fnType == "" {
		fnType = strings.TrimSpace(task.Assignee)
	}
	if fnType == "" {
		writeTerminal(task, workspacePath, store, api, meta.TaskStatusFailed,
			runMetadata,
			`{"error":"no fn: label or assignee on function task"}`, 0)
		return
	}
	handler := Lookup(fnType)
	if handler == nil {
		writeTerminal(task, workspacePath, store, api, meta.TaskStatusFailed, runMetadata,
			fmt.Sprintf(`{"error":"function type %q not registered"}`, fnType), 0)
		return
	}

	log.Printf("[function-runner] executing fn:%s task %s", fnType, task.ID)
	ctx := FunctionContext{Task: task}
	result, err := handler(ctx)
	if err != nil {
		log.Printf("[function-runner] fn:%s task %s failed: %v", fnType, task.ID, err)
		payload, _ := json.Marshal(map[string]string{"error": err.Error()})
		writeTerminal(task, workspacePath, store, api, meta.TaskStatusFailed, runMetadata, string(payload), ctx.CostTokens)
		return
	}

	var resultJSON string
	if result != nil {
		data, jsonErr := json.Marshal(result)
		if jsonErr != nil {
			resultJSON = fmt.Sprintf(`{"error":"marshal result: %s"}`, jsonErr)
		} else {
			resultJSON = string(data)
		}
	} else {
		resultJSON = "null"
	}

	log.Printf("[function-runner] fn:%s task %s completed", fnType, task.ID)
	writeTerminal(task, workspacePath, store, api, meta.TaskStatusCompleted, runMetadata, resultJSON, ctx.CostTokens)
}

// writeTerminal persists the terminal state and fires completion hooks.
func writeTerminal(task meta.Task, workspacePath string, store *meta.TaskStore, api *API,
	status meta.TaskStatus, runMetadata meta.TaskRun, resultJSON string, costTokens int64) {
	now := time.Now().UTC()
	var run meta.TaskRun
	var closedBy *meta.ClosedBy
	if store != nil {
		var auditErr error
		runMetadata.TaskID, runMetadata.Kind = task.ID, meta.TaskRunExecution
		run, auditErr = store.TaskRuns().Create(workspacePath, runMetadata)
		if auditErr == nil {
			runStatus := meta.TaskRunCompleted
			evidenceKind := "function_result"
			errorText := ""
			if status == meta.TaskStatusFailed {
				runStatus = meta.TaskRunFailed
				evidenceKind = "function_error"
				errorText = resultJSON
			} else {
				closedBy = &meta.ClosedBy{Kind: "function_evidence", Verdict: "passed"}
			}
			run, auditErr = store.TaskRuns().Finish(run.ID, runStatus, []meta.CompletionEvidence{{
				Kind: evidenceKind, Summary: "Function runner produced a structured result.",
			}}, nil, closedBy, errorText)
		}
		if auditErr != nil && status == meta.TaskStatusCompleted {
			status = meta.TaskStatusFailed
			resultJSON = fmt.Sprintf(`{"error":"completion audit failed: %s"}`, auditErr)
			closedBy = nil
		}
		_ = store.Mutate(workspacePath, func(cfg *meta.TasksConfig) bool {
			for i := range cfg.Tasks {
				if cfg.Tasks[i].ID == task.ID {
					cfg.Tasks[i].Status = status
					cfg.Tasks[i].Result = resultJSON
					cfg.Tasks[i].CostTokens = costTokens
					cfg.Tasks[i].UpdatedAt = now
					cfg.Tasks[i].CompletedAt = &now
					cfg.Tasks[i].ClosedBy = closedBy
					if status == meta.TaskStatusCompleted {
						cfg.Tasks[i].Replies = append(cfg.Tasks[i].Replies, meta.Reply{
							Author: meta.Author{Kind: "system", Name: "completion-gate"},
							Text:   fmt.Sprintf("完成审计：TaskRun `%s`，Function Evidence 已记录。", run.ID),
							Mode:   meta.ModePureComment, CreatedAt: now,
						})
					}
					return true
				}
			}
			return false
		})
	}
	if api != nil {
		api.NotifyCompletion(CompletionEvent{
			TaskID:      task.ID,
			Status:      status,
			Result:      resultJSON,
			CostTokens:  costTokens,
			CompletedAt: now,
		})
	}
}

// ── built-in sample handlers ─────────────────────────────────────────────────

func init() {
	// core.noop: trivial handler to prove the function pipeline works.
	// Returns the task title as confirmation. Token cost = 0.
	RegisterFunction("core.noop", func(ctx FunctionContext) (any, error) {
		return map[string]string{
			"status": "ok",
			"task":   ctx.Task.Title,
		}, nil
	})
}
