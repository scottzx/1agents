package sediment

import (
	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/retro"
)

// Result is the output of one full pass over the 沉淀链: the candidate set at
// each downstream layer. It is the "可运行最小链路" deliverable — one call that
// walks 经验 → 技能 → 任务模板 → 项目模板 and returns every layer's proposals.
type Result struct {
	Skills          []SkillCandidate
	TaskTemplates   []TaskTemplateCandidate
	ProjectTemplate ProjectTemplateCandidate
}

// Run executes the default chain end-to-end:
//
//	复盘 (retro.Retrospective) ──PromoteExperiences──▶ 技能卡候选
//	既有技能卡 (agent.Skill)   ──PromoteSkills────────▶ 任务模板候选
//	任务模板候选               ──PromoteTaskTemplates──▶ 项目模板候选
//
// The two seed inputs are the project retrospective (the fresh 经验 source) and
// the existing skill library (skills already loaded by #187). Promotion of the
// retro's distilled skill candidates into the task-template layer is deliberately
// left to a later sub-stage (4.1 review/mint step): only *minted* skills feed the
// task-template edge here, keeping the minimal chain honest about what is a
// reviewed asset vs. a raw proposal.
func Run(r retro.Retrospective, existingSkills []agent.Skill) Result {
	tts := PromoteSkills(existingSkills)
	return Result{
		Skills:          PromoteExperiences(r),
		TaskTemplates:   tts,
		ProjectTemplate: PromoteTaskTemplates(r.Project.Name, tts),
	}
}
