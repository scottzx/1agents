package radio

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// Pipeline stages (also used as Milestone labels on dispatched tasks).
const (
	StageSummarize  = "summarize"
	StageTranscript = "transcript"
	StageSynthesize = "synthesize"
)

// TTSFunctionType is the function-handler key for stage 3. It lives in the
// "radio." namespace and matches manifest.taskTypes.
const TTSFunctionType = "radio.tts_synthesize"

// businessRef builds the binding seam for an episode: "radio:episode:<id>".
func businessRef(episodeID int64) string {
	return fmt.Sprintf("%s:episode:%d", AppID, episodeID)
}

// parseEpisodeRef extracts the episode id from a "radio:episode:<id>" ref.
// ok=false when ref does not belong to this app.
func parseEpisodeRef(ref string) (int64, bool) {
	const prefix = AppID + ":episode:"
	if !strings.HasPrefix(ref, prefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(ref, prefix), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// StartPipeline issues the 3-stage pipeline for an episode through the North
// Task API. All three tasks are bound to business_ref "radio:episode:<id>" and
// chained with DependsOn so the executor-agnostic scheduler advances them:
//
//  1. summarize  (agent)    — read source, write a tight summary
//  2. transcript (agent)    — turn the summary into a spoken-radio script
//  3. synthesize (function) — radio.tts_synthesize → audio artifact, token≈0
//
// The app NEVER embeds an agent: it only calls taskapi. Returns the task IDs in
// stage order.
func (a *App) StartPipeline(episodeID int64, workspace string) ([]string, error) {
	ref := businessRef(episodeID)
	title := fmt.Sprintf("电台剧集 #%d", episodeID)

	// Stage 1 — 总结 (agent). Issued first so we have its id for the chain.
	summaryIDs, err := a.api.IssueTasksFromBusiness(AppID, ref, StageSummarize, []taskapi.DispatchSpec{{
		Title:              title + " · 内容总结",
		Description:        "阅读来源内容，产出一段 200 字以内的精炼总结，作为本期电台的选题概要。",
		AcceptanceCriteria: "总结准确、信息密度高、可直接作为后续口播脚本的基础。",
		Executor:           meta.TaskExecutorAgent,
		WorkspacePath:      workspace,
	}})
	if err != nil {
		return nil, fmt.Errorf("radio: dispatch summarize: %w", err)
	}
	summaryID := summaryIDs[0]

	// Stage 2 — 逐字稿 (agent), depends on stage 1.
	transcriptIDs, err := a.api.IssueTasksFromBusiness(AppID, ref, StageTranscript, []taskapi.DispatchSpec{{
		Title:              title + " · 生成逐字稿",
		Description:        "基于上一阶段的总结，撰写一段可直接朗读的电台口播逐字稿（自然、口语化）。",
		AcceptanceCriteria: "逐字稿可被 TTS 直接合成；语气自然、断句清晰。",
		Executor:           meta.TaskExecutorAgent,
		DependsOn:          []string{summaryID},
		WorkspacePath:      workspace,
	}})
	if err != nil {
		return nil, fmt.Errorf("radio: dispatch transcript: %w", err)
	}
	transcriptID := transcriptIDs[0]

	// Stage 3 — TTS 合成 (function), depends on stage 2.
	ttsIDs, err := a.api.IssueTasksFromBusiness(AppID, ref, StageSynthesize, []taskapi.DispatchSpec{{
		Title:         title + " · TTS 合成",
		Description:   "把逐字稿合成为音频，落到工作区文件面。",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  TTSFunctionType,
		DependsOn:     []string{transcriptID},
		WorkspacePath: workspace,
	}})
	if err != nil {
		return nil, fmt.Errorf("radio: dispatch tts: %w", err)
	}

	_ = a.store.SetStatus(episodeID, StatusSummarizing)
	return []string{summaryID, transcriptID, ttsIDs[0]}, nil
}
