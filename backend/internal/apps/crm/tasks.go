package crm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// liveAPI is the North Task API handle captured in appkit.OnInit. HTTP handlers
// dispatch through it. nil until the server wires apps (RunInits).
var liveAPI *taskapi.API

// ── #342 enrichment (executor=function) ──────────────────────────────────────

// enrichResult is the JSON shape written back to task.result by enrichHandler.
type enrichResult struct {
	Kind      string `json:"kind"` // "crm.enrich" — discriminates writeback path
	LeadID    string `json:"leadId"`
	Company   string `json:"company"`
	Industry  string `json:"industry"`
	Size      string `json:"size"`
	ScoreBump int    `json:"scoreBump"`
	Note      string `json:"note"`
}

// enrichHandler is the deterministic external-enrichment worker (#342). In Phase 1
// it stubs the external call deterministically (token≈0) so the pipeline is
// provable end-to-end; a real handler would call an external company-data API and
// fill CostTokens. The lead id is carried on the task's business_ref.
func enrichHandler(ctx taskapi.FunctionContext) (any, error) {
	ref := ctx.Task.BusinessRef
	leadID, ok := LeadIDFromRef(ref)
	if !ok {
		return nil, fmt.Errorf("crm.enrich: task %s has no crm:lead: business_ref (got %q)", ctx.Task.ID, ref)
	}
	company := ""
	if pkgStore != nil {
		if lead, found, _ := pkgStore.GetLead(leadID); found {
			if c, cfound, _ := pkgStore.GetContact(lead.ContactID); cfound {
				company = c.Company
			}
		}
	}
	// Deterministic stub enrichment — replace with a real data provider call.
	industry := deriveIndustry(company)
	return enrichResult{
		Kind:      TaskTypeEnrich,
		LeadID:    leadID,
		Company:   company,
		Industry:  industry,
		Size:      "11-50",
		ScoreBump: 10,
		Note:      "外部富集(stub):行业=" + industry,
	}, nil
}

// deriveIndustry is a deterministic stand-in for an external classifier.
func deriveIndustry(company string) string {
	c := strings.ToLower(company)
	switch {
	case strings.Contains(c, "tech") || strings.Contains(c, "ai") || strings.Contains(c, "科技"):
		return "科技"
	case strings.Contains(c, "bank") || strings.Contains(c, "金融"):
		return "金融"
	case company == "":
		return "未知"
	default:
		return "通用"
	}
}

// DispatchEnrich queues a function enrichment task for a lead (#342). Returns the
// task id. Routed through IssueTasksFromBusiness so business_ref is set.
func DispatchEnrich(workspacePath, leadID string) (string, error) {
	if liveAPI == nil {
		return "", fmt.Errorf("crm: task API not wired")
	}
	ids, err := liveAPI.IssueTasksFromBusiness(AppID, LeadRef(leadID), "enrich", []taskapi.DispatchSpec{
		{
			Title:         "富集线索 " + leadID,
			Description:   "对该线索的关联公司做外部数据富集(行业/规模)。",
			Executor:      meta.TaskExecutorFunction,
			FunctionType:  TaskTypeEnrich,
			WorkspacePath: workspacePath,
		},
	})
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// ── #341 lead mining / scoring (executor=agent) ──────────────────────────────

// DispatchScore queues an agent task that mines source context for商机线索 and
// scores the lead, in the digest ACP analysis style (#341). The completion hook
// parses the agent's JSON result and updates crm_lead.score/notes.
func DispatchScore(workspacePath, leadID, sourceContext string) (string, error) {
	if liveAPI == nil {
		return "", fmt.Errorf("crm: task API not wired")
	}
	desc := "分析以下来源内容,判断其商机价值,给出 0-100 的评分与建议下一步。\n\n" +
		"来源内容:\n" + sourceContext + "\n\n" +
		"输出 JSON:{\"kind\":\"crm.score\",\"score\":<0-100>,\"reason\":\"...\",\"nextStep\":\"...\"}"
	ids, err := liveAPI.IssueTasksFromBusiness(AppID, LeadRef(leadID), "score", []taskapi.DispatchSpec{
		{
			Title:              "挖掘/打分线索 " + leadID,
			Description:        desc,
			AcceptanceCriteria: "结果是合法 JSON,含 score(0-100)整数字段。",
			Executor:           meta.TaskExecutorAgent,
			WorkspacePath:      workspacePath,
		},
	})
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// scoreResult is the JSON shape an agent score task is expected to emit.
type scoreResult struct {
	Kind     string `json:"kind"` // "crm.score"
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
	NextStep string `json:"nextStep"`
}

// ── #343 follow / drop human decision (executor=human) ───────────────────────

// DispatchDecision creates a human-executor task: a 跟进/放弃 decision gate for a
// lead (#343). When the human completes it, the completion hook advances the
// lead's stage (跟进 → contacted, 放弃 → dropped) based on the recorded outcome.
//
// The intended outcome is embedded in the business_ref-adjacent result by the
// human-complete path; here we encode it in the task milestone so the hook can
// route it: milestone "decide:contacted" or "decide:dropped".
func DispatchDecision(workspacePath, leadID, outcomeStage string) (string, error) {
	if liveAPI == nil {
		return "", fmt.Errorf("crm: task API not wired")
	}
	label := "跟进"
	if outcomeStage == StageDropped {
		label = "放弃"
	}
	id, err := liveAPI.DispatchTask(AppID, taskapi.DispatchSpec{
		Title:         label + "线索 " + leadID,
		Description:   "对该线索做跟进/放弃决策。完成即推进漏斗阶段。",
		Executor:      meta.TaskExecutorHuman,
		BusinessRef:   LeadRef(leadID),
		Milestone:     "decide:" + outcomeStage,
		WorkspacePath: workspacePath,
	})
	return id, err
}

// ── completion writeback hook ────────────────────────────────────────────────

// completionHook is fired by the kernel when ANY task reaches a terminal state.
// It claims only crm: business_refs and writes results back into crm_lead.
// Non-crm events return immediately (R1/R5).
func completionHook(ev taskapi.CompletionEvent) {
	if pkgStore == nil {
		return
	}
	// Resolve the task to read its business_ref + milestone (the event carries
	// neither). Safe: QueryTask is read-only.
	if liveAPI == nil {
		return
	}
	task, ok, err := liveAPI.QueryTask(ev.TaskID)
	if err != nil || !ok {
		return
	}
	leadID, isLead := LeadIDFromRef(task.BusinessRef)
	if !isLead {
		return // not ours
	}

	switch {
	case task.Executor == meta.TaskExecutorHuman:
		// #343: human follow/drop decision completed → advance stage.
		if ev.Status != meta.TaskStatusCompleted {
			return
		}
		stage := strings.TrimPrefix(task.Milestone, "decide:")
		if stage == "" || stage == task.Milestone {
			stage = StageContacted
		}
		_, _ = pkgStore.UpdateLeadStage(leadID, stage)

	case taskapi.ExtractFunctionType(task.Labels) == TaskTypeEnrich:
		// #342: enrichment function done → bump score, append note.
		if ev.Status != meta.TaskStatusCompleted {
			return
		}
		var r enrichResult
		if json.Unmarshal([]byte(ev.Result), &r) == nil {
			if lead, found, _ := pkgStore.GetLead(leadID); found {
				_, _ = pkgStore.UpdateLeadScore(leadID, lead.Score+r.ScoreBump, r.Note)
			}
		}

	case task.Executor == meta.TaskExecutorAgent:
		// #341: agent mining/scoring done → set score + reason notes.
		if ev.Status != meta.TaskStatusCompleted {
			return
		}
		var r scoreResult
		if json.Unmarshal([]byte(ev.Result), &r) == nil && r.Score > 0 {
			note := r.Reason
			if r.NextStep != "" {
				note += " · 下一步:" + r.NextStep
			}
			_, _ = pkgStore.UpdateLeadScore(leadID, r.Score, note)
		}
	}
}
