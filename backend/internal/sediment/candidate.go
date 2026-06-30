package sediment

// Candidate is the common shape carried between layers. Each PromoteX emits the
// downstream layer's candidates from the upstream layer's artifacts. A candidate
// is a *proposal*: it is reviewed (by a human or a downstream agent) before being
// minted into the real asset (an agent.Skill, a stored task template, …).
//
// The four concrete candidate types embed Candidate so they share provenance and
// the Layer tag while keeping their layer-specific payload.
type Candidate struct {
	// Layer is which rung this candidate targets (the To of the stage that
	// produced it).
	Layer Layer
	// Title is a short human label for the proposed asset.
	Title string
	// Rationale explains why the chain surfaced this candidate (which upstream
	// evidence drove it) — the reviewer reads this to accept/reject.
	Rationale string
	// SourceIDs are stable identifiers of the upstream artifacts this candidate
	// was distilled from (project ids, skill names, …) for provenance/dedup.
	SourceIDs []string
}

// SkillCandidate is a proposed 技能卡 distilled from 复盘经验 (Experience → Skill).
type SkillCandidate struct {
	Candidate
	// Description seeds the skill card's frontmatter description.
	Description string
}

// TaskTemplateCandidate is a proposed 任务模板 aggregated from 技能卡
// (Skill → TaskTemplate). It names the skills a task of this shape should bind.
type TaskTemplateCandidate struct {
	Candidate
	// Skills are the skill names this task template would wire in.
	Skills []string
	// Checks are minimal acceptance/verification items for the task shape.
	Checks []string
}

// ProjectTemplateCandidate is the apex asset (TaskTemplate → ProjectTemplate):
// a reusable project shape carrying its skills, task ordering, and check items.
type ProjectTemplateCandidate struct {
	Candidate
	// Skills is the union of skills the constituent task templates need.
	Skills []string
	// TaskOrder is the proposed task sequence (前后依赖) by template title.
	TaskOrder []string
	// Checks is the merged, deduped set of check items across the templates.
	Checks []string
}
