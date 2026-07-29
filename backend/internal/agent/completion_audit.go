package agent

import (
	"fmt"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// recordDerivedCompletion closes a Task whose completion is decided outside an
// agent runtime (for example an IM approval or a container whose children all
// completed). It keeps those legacy paths behind the same TaskRun completion
// gate as interactive and headless execution.
func recordDerivedCompletion(store *TasksStore, workspacePath, taskID, closedKind, evidenceKind, summary string) (*Task, error) {
	runs := store.TaskRuns()
	run, err := runs.Create(workspacePath, meta.TaskRun{
		TaskID: taskID,
		Kind:   meta.TaskRunExecution,
	})
	if err == nil {
		run, err = runs.Finish(run.ID, meta.TaskRunCompleted, []meta.CompletionEvidence{{
			Kind: evidenceKind, Summary: summary,
		}}, nil, &meta.ClosedBy{Kind: closedKind, Verdict: "passed"}, "")
	}
	if err != nil {
		_ = store.Mutate(workspacePath, func(cfg *TasksConfig) bool {
			for i := range cfg.Tasks {
				if cfg.Tasks[i].ID != taskID {
					continue
				}
				cfg.Tasks[i].Status = TaskStatusFailed
				cfg.Tasks[i].CompletedAt = nil
				cfg.Tasks[i].ClosedBy = nil
				cfg.Tasks[i].Summary = "completion audit failed: " + err.Error()
				cfg.Tasks[i].UpdatedAt = time.Now().UTC()
				return true
			}
			return false
		})
		return nil, fmt.Errorf("completion audit failed: %w", err)
	}

	now := time.Now().UTC()
	if err := store.Mutate(workspacePath, func(cfg *TasksConfig) bool {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID != taskID {
				continue
			}
			cfg.Tasks[i].ClosedBy = run.ClosedBy
			cfg.Tasks[i].Replies = append(cfg.Tasks[i].Replies, Reply{
				Author: Author{Kind: "system", Name: "completion-gate"},
				Text:   fmt.Sprintf("完成审计：TaskRun `%s`，%s Evidence 已记录。", run.ID, evidenceKind),
				Mode:   ModePureComment, CreatedAt: now,
			})
			cfg.Tasks[i].UpdatedAt = now
			return true
		}
		return false
	}); err != nil {
		return nil, err
	}
	task, ok, err := store.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("task not found after completion audit: %s", taskID)
	}
	return &task, nil
}
