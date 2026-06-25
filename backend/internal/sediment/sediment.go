// Package sediment is the 技能沉淀链总纲 (#143): the explicit, compounding
// pipeline that turns each project's experience into reusable assets so a single
// operator gets stronger over time.
//
// 沉淀链 (issue #143 顺序):
//
//	提示词+工具 → 技能卡 → 任务模板 → 项目模板/架构
//
// This package does NOT re-implement the upstream stages — those already exist:
//   - 复盘经验 comes from internal/retro (#144, Retrospective.Experiences).
//   - 技能卡 is agent.Skill (#187 loader, #188 absorb).
//   - 任务/项目 are meta.Task / meta.Project (#135 / #141).
//
// What sediment adds is the connective tissue the epic asked for:
//  1. 各层制品类型 — Layer + the typed Candidate carried between layers.
//  2. 「上一层→下一层」提炼/晋升关系 — Chain.Stages declares the edges.
//  3. 最小链式提炼函数 — the PromoteX functions that derive the next layer's
//     candidates from the previous layer's artifacts.
//
// Design (简单优先, see CLAUDE.md §2):
//   - Promotion is rule-based and pure (no LLM, no IO, no vector store). Each
//     PromoteX is a deterministic transform so it is trivially unit-testable.
//   - Promotion produces *candidates*, never finished assets: a candidate is a
//     proposal a human (or a downstream agent) reviews before it becomes a real
//     Skill / task template / project template. The chain surfaces material; it
//     does not silently mint assets.
//   - This is a 总纲 (master outline): a runnable minimal chain + the type
//     skeleton, not a one-shot full build. Sub-stages 4.1/4.2/4.3 refine each
//     edge later.
package sediment

// Layer is one rung of the 沉淀链. Ordered from rawest (Experience) to most
// valuable (ProjectTemplate) — the issue notes 项目架构 is 最值钱的.
type Layer string

const (
	// LayerExperience — 复盘经验: lessons distilled from a project retrospective
	// (#144). The raw seed material the chain refines upward.
	LayerExperience Layer = "experience"
	// LayerSkill — 技能卡: a callable capability (提示词+工具 → 可快速调用的技能).
	// Maps to agent.Skill.
	LayerSkill Layer = "skill"
	// LayerTaskTemplate — 任务模板: a reusable task shape (a recurring kind of
	// work that bundles which skills it needs).
	LayerTaskTemplate Layer = "task_template"
	// LayerProjectTemplate — 项目模板/架构: the apex asset — a reusable project
	// shape carrying its skills + task ordering + check items, so a similar
	// project starts with "有哪些技能 + 哪些任务前后依赖 + 检查项" built in.
	LayerProjectTemplate Layer = "project_template"
)

// Stage is one directed edge of the chain: 提炼/晋升 from one layer to the next.
type Stage struct {
	// From is the upstream (source) layer.
	From Layer
	// To is the downstream (refined) layer this stage promotes into.
	To Layer
	// Desc is a short human description of what the promotion extracts.
	Desc string
}

// Chain is the master outline (总纲): the ordered layers and the promotion edges
// between them. DefaultChain wires the canonical issue #143 pipeline.
type Chain struct {
	Stages []Stage
}

// DefaultChain returns the canonical 沉淀链:
//
//	经验 → 技能卡 → 任务模板 → 项目模板/架构
func DefaultChain() Chain {
	return Chain{Stages: []Stage{
		{From: LayerExperience, To: LayerSkill, Desc: "复盘经验提炼为技能卡候选"},
		{From: LayerSkill, To: LayerTaskTemplate, Desc: "技能卡聚合为任务模板候选"},
		{From: LayerTaskTemplate, To: LayerProjectTemplate, Desc: "任务模板编排为项目模板/架构候选"},
	}}
}

// Layers returns the chain's layers in promotion order, derived from Stages.
// (Source of the first stage, then each stage's target.)
func (c Chain) Layers() []Layer {
	if len(c.Stages) == 0 {
		return nil
	}
	out := []Layer{c.Stages[0].From}
	for _, s := range c.Stages {
		out = append(out, s.To)
	}
	return out
}

// Next reports the layer that l promotes into, and whether such an edge exists.
func (c Chain) Next(l Layer) (Layer, bool) {
	for _, s := range c.Stages {
		if s.From == l {
			return s.To, true
		}
	}
	return "", false
}
