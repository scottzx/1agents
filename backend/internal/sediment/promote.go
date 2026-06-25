package sediment

import (
	"sort"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/retro"
)

// PromoteExperiences is the first chain edge (Experience → Skill): it turns a
// project's 复盘经验 (retro output) into 技能卡 candidates.
//
// Rule (minimal, 无 LLM): each retrospective experience line becomes one skill
// candidate, titled from the project so the reviewer knows where it came from.
// Lines that are pure quantitative noise ("完成率 N%") carry no reusable method,
// so they are filtered — the chain proposes capabilities, not statistics.
func PromoteExperiences(r retro.Retrospective) []SkillCandidate {
	var out []SkillCandidate
	for _, exp := range r.Experiences {
		exp = strings.TrimSpace(exp)
		if exp == "" || isStatLine(exp) {
			continue
		}
		out = append(out, SkillCandidate{
			Candidate: Candidate{
				Layer:     LayerSkill,
				Title:     r.Project.Name + " · " + firstClause(exp),
				Rationale: "源自复盘经验：" + exp,
				SourceIDs: []string{"retro-" + r.Project.ID},
			},
			Description: exp,
		})
	}
	return out
}

// PromoteSkills is the second chain edge (Skill → TaskTemplate): it aggregates a
// set of 技能卡 into 任务模板 candidates.
//
// Rule (minimal): skills sharing a leading namespace (the segment before the
// first "-" / "·" / " " in the name, e.g. "code-review" → "code") are grouped
// into one task-template candidate that binds those skills. A lone skill still
// yields a singleton template so nothing is dropped. Each template seeds a
// "完成后核验" check from the verification-before-completion convention.
func PromoteSkills(skills []agent.Skill) []TaskTemplateCandidate {
	groups := map[string][]agent.Skill{}
	var order []string
	for _, sk := range skills {
		if strings.TrimSpace(sk.Name) == "" {
			continue
		}
		key := namespace(sk.Name)
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], sk)
	}
	sort.Strings(order)

	var out []TaskTemplateCandidate
	for _, key := range order {
		grp := groups[key]
		names := make([]string, 0, len(grp))
		ids := make([]string, 0, len(grp))
		for _, sk := range grp {
			names = append(names, sk.Name)
			ids = append(ids, "skill-"+sk.Name)
		}
		sort.Strings(names)
		out = append(out, TaskTemplateCandidate{
			Candidate: Candidate{
				Layer:     LayerTaskTemplate,
				Title:     key + " 任务模板",
				Rationale: "聚合技能卡：" + strings.Join(names, ", "),
				SourceIDs: ids,
			},
			Skills: names,
			Checks: []string{"完成前自检（verification-before-completion）"},
		})
	}
	return out
}

// PromoteTaskTemplates is the apex chain edge (TaskTemplate → ProjectTemplate):
// it orchestrates 任务模板 into a 项目模板/架构 candidate.
//
// Rule (minimal): the templates are composed into one project shape — TaskOrder
// is their titles in input order (前后依赖 placeholder; refining real
// dependencies is downstream), Skills is the deduped union of every template's
// skills, and Checks is the deduped union of every template's checks. This is
// the "执行类似任务时自带 有哪些技能 + 哪些任务前后依赖 + 检查项" payload the
// issue calls 最值钱的.
func PromoteTaskTemplates(name string, templates []TaskTemplateCandidate) ProjectTemplateCandidate {
	pc := ProjectTemplateCandidate{
		Candidate: Candidate{Layer: LayerProjectTemplate, Title: name + " 项目模板"},
	}
	seenSkill := map[string]bool{}
	seenCheck := map[string]bool{}
	var ids []string
	for _, t := range templates {
		pc.TaskOrder = append(pc.TaskOrder, t.Title)
		ids = append(ids, t.SourceIDs...)
		for _, s := range t.Skills {
			if !seenSkill[s] {
				seenSkill[s] = true
				pc.Skills = append(pc.Skills, s)
			}
		}
		for _, c := range t.Checks {
			if !seenCheck[c] {
				seenCheck[c] = true
				pc.Checks = append(pc.Checks, c)
			}
		}
	}
	sort.Strings(pc.Skills)
	pc.SourceIDs = ids
	pc.Rationale = "编排任务模板：" + strings.Join(pc.TaskOrder, " → ")
	return pc
}

// isStatLine reports whether an experience line is a bare quantitative summary
// (carries no reusable method), e.g. "共 N 个任务，完成 M 个（完成率 X%）。".
func isStatLine(s string) bool {
	return strings.Contains(s, "完成率")
}

// firstClause returns the text up to the first Chinese/ASCII sentence break, so
// a long experience line yields a compact skill title.
func firstClause(s string) string {
	if i := strings.IndexAny(s, "，,。:："); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// namespace extracts the grouping key of a skill name: the segment before the
// first separator ("-", "·", or space). Names without a separator group under
// themselves.
func namespace(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexAny(name, "-· "); i > 0 {
		return name[:i]
	}
	return name
}
