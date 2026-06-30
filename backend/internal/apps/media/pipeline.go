package media

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// liveAPI / liveStore are injected by appkit.OnInit at startup. All task
// dispatch goes through the API; the store is used for the human decision-gate
// completion path (the API exposes no public status setter). Both nil before init.
var (
	apiMu     sync.RWMutex
	liveAPI   *taskapi.API
	liveStore *meta.TaskStore
)

func setRuntime(a *taskapi.API, s *meta.TaskStore) {
	apiMu.Lock()
	liveAPI = a
	liveStore = s
	apiMu.Unlock()
}

func runtime() (*taskapi.API, *meta.TaskStore, error) {
	apiMu.RLock()
	defer apiMu.RUnlock()
	if liveAPI == nil {
		return nil, nil, fmt.Errorf("media: task API not initialized")
	}
	return liveAPI, liveStore, nil
}

// BusinessRef builds the canonical business_ref for a media entity.
// Format: "media:<entity>:<id>" (e.g. "media:project:abc", "media:material:xyz").
func BusinessRef(entity, id string) string {
	return fmt.Sprintf("%s:%s:%s", AppID, entity, id)
}

// LaunchProcessingPipeline issues the mixed-executor processing pipeline for one
// material (#337): a function stage (silence detect) → an agent stage (smart
// edit/段落取舍) → a human stage (final approval gate). Stages are wired with
// DependsOn so the executor-agnostic scheduler advances them in order.
//
// Uses IssueTasksFromBusiness with business_ref "media:material:<id>".
// Returns the created task IDs in pipeline order [silence, edit, approve].
func LaunchProcessingPipeline(projectID, materialID string) ([]string, error) {
	a, _, err := runtime()
	if err != nil {
		return nil, err
	}
	cp, ok, err := GetProject(projectID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("media: content project %q not found", projectID)
	}
	mat, ok, err := GetMaterial(materialID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("media: material %q not found", materialID)
	}

	ws := cp.Workspace
	ref := BusinessRef("material", materialID)

	specs := []taskapi.DispatchSpec{
		// Stage 1 — function: deterministic silence detection (token≈0).
		{
			Title: "静音检测 · " + shortID(materialID),
			Description: fmt.Sprintf("对素材做静音/语音分段检测,产出候选段落。\nmaterial=%s duration=%g",
				materialID, mat.Duration),
			Executor:      meta.TaskExecutorFunction,
			FunctionType:  FnSilenceDetect,
			Milestone:     "silence_detect",
			WorkspacePath: ws,
		},
	}
	ids, err := a.IssueTasksFromBusiness(AppID, ref, "", specs)
	if err != nil {
		return ids, fmt.Errorf("media: issue silence stage: %w", err)
	}
	silenceID := ids[0]

	// Stage 2 — agent: smart edit / 多素材取舍 (柔, → ACP). Depends on stage 1.
	editID, err := a.DispatchTask(AppID, taskapi.DispatchSpec{
		Title:              "智能剪辑取舍 · " + shortID(materialID),
		Description:        "基于静音分段,挑选保留段落、给出剪辑脚本建议。\nmaterial=" + materialID,
		AcceptanceCriteria: "产出 keep/drop 段落建议与剪辑脚本。",
		Executor:           meta.TaskExecutorAgent,
		BusinessRef:        ref,
		Milestone:          "edit",
		DependsOn:          []string{silenceID},
		WorkspacePath:      ws,
		Target:             &meta.TaskTargetSpec{Cwd: ws},
	})
	if err != nil {
		return ids, fmt.Errorf("media: dispatch edit stage: %w", err)
	}
	ids = append(ids, editID)

	// Stage 3 — human: final 金句/段落取舍 approval (裁). Depends on stage 2.
	approveID, err := a.DispatchTask(AppID, taskapi.DispatchSpec{
		Title:         "段落取舍终审 · " + shortID(materialID),
		Description:   "人工确认最终保留段落 / 金句。\nmaterial=" + materialID,
		Executor:      meta.TaskExecutorHuman,
		BusinessRef:   ref,
		Milestone:     "approve",
		DependsOn:     []string{editID},
		WorkspacePath: ws,
	})
	if err != nil {
		return ids, fmt.Errorf("media: dispatch approve stage: %w", err)
	}
	ids = append(ids, approveID)

	_ = SetMaterialStage(materialID, "processing")
	return ids, nil
}

// LaunchRetrimTask issues a single function=trim task from a human segment
// decision (#338: 勾选段落 → 决策回写触发重剪任务).
func LaunchRetrimTask(projectID, materialID, inPath, outPath string, start, end float64) (string, error) {
	a, _, err := runtime()
	if err != nil {
		return "", err
	}
	cp, ok, err := GetProject(projectID)
	if err != nil || !ok {
		return "", fmt.Errorf("media: content project %q not found", projectID)
	}
	id, err := a.DispatchTask(AppID, taskapi.DispatchSpec{
		Title: "重剪 · " + shortID(materialID),
		Description: fmt.Sprintf("按确认段落重新裁剪素材。\nmaterial=%s in=%s out=%s start=%g end=%g",
			materialID, inPath, outPath, start, end),
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  FnTrim,
		BusinessRef:   BusinessRef("material", materialID),
		Milestone:     "trim",
		WorkspacePath: cp.Workspace,
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// ResolveHumanTask completes a human-executor decision-gate task with a verdict
// (#338). The API exposes no public status setter, so this mirrors the kernel's
// function-runner terminal path: Mutate the task to completed + result, then
// fire completion hooks (consumed by the writeback hook to advance domain state).
func ResolveHumanTask(taskID, verdict string, payload map[string]any) error {
	a, store, err := runtime()
	if err != nil {
		return err
	}
	task, ok, err := a.QueryTask(taskID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("media: task %q not found", taskID)
	}
	if task.Executor != meta.TaskExecutorHuman {
		return fmt.Errorf("media: task %q is not a human task", taskID)
	}
	ws, err := workspaceForTask(task)
	if err != nil {
		return err
	}

	result := map[string]any{"verdict": verdict}
	for k, v := range payload {
		result[k] = v
	}
	resultJSON, _ := json.Marshal(result)

	now := time.Now().UTC()
	if store != nil {
		err = store.Mutate(ws, func(cfg *meta.TasksConfig) bool {
			for i := range cfg.Tasks {
				if cfg.Tasks[i].ID == taskID {
					cfg.Tasks[i].Status = meta.TaskStatusCompleted
					cfg.Tasks[i].IssueState = meta.IssueClosed
					cfg.Tasks[i].Result = string(resultJSON)
					cfg.Tasks[i].UpdatedAt = now
					cfg.Tasks[i].CompletedAt = &now
					return true
				}
			}
			return false
		})
		if err != nil {
			return fmt.Errorf("media: complete human task: %w", err)
		}
	}
	a.NotifyCompletion(taskapi.CompletionEvent{
		TaskID:      taskID,
		Status:      meta.TaskStatusCompleted,
		Result:      string(resultJSON),
		CostTokens:  0,
		CompletedAt: now,
	})
	return nil
}

// workspaceForTask resolves the workspace path for a task via its business_ref
// (media:material:<id> → material.project_id → content project workspace).
func workspaceForTask(task meta.Task) (string, error) {
	entity, id, ok := parseBusinessRef(task.BusinessRef)
	if !ok {
		return "", fmt.Errorf("media: task %q has no media business_ref", task.ID)
	}
	switch entity {
	case "material":
		mat, found, err := GetMaterial(id)
		if err != nil || !found {
			return "", fmt.Errorf("media: material %q for task not found", id)
		}
		cp, found, err := GetProject(mat.ProjectID)
		if err != nil || !found {
			return "", fmt.Errorf("media: project for material %q not found", id)
		}
		return cp.Workspace, nil
	case "project":
		cp, found, err := GetProject(id)
		if err != nil || !found {
			return "", fmt.Errorf("media: project %q not found", id)
		}
		return cp.Workspace, nil
	}
	return "", fmt.Errorf("media: unknown business_ref entity %q", entity)
}

// parseBusinessRef splits "media:<entity>:<id>". ok=false when it doesn't match.
func parseBusinessRef(ref string) (entity, id string, ok bool) {
	const p = AppID + ":"
	if len(ref) <= len(p) || ref[:len(p)] != p {
		return "", "", false
	}
	rest := ref[len(p):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == ':' {
			return rest[:i], rest[i+1:], true
		}
	}
	return "", "", false
}

// shortID returns a short readable prefix of an id for titles.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
