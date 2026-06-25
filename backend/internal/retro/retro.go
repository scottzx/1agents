// Package retro is the 复盘沉淀 (#144) entry of the 技能沉淀链 (epic #143):
// when a project is archived (#141 阶段性完成归档), it汇总该项目的任务/决策/产出,
// 生成一份复盘记录, 并落进 kwiki (#191) 的 wiki 层, 喂给下游技能沉淀。
//
// 设计 (简单优先, 见 CLAUDE.md §2):
//   - 核心是纯函数 Summarize: (项目元信息 + 任务清单) → Retrospective, 不碰 IO,
//     便于单测。
//   - Render 把 Retrospective 渲染成 Markdown; ToInboxItem 映射成 kwiki.InboxItem,
//     与 internal/research 落库的做法一致 (本包不重写知识压缩, 复用 kwiki.Ingest)。
//   - Archive 是挂在归档流程上的薄胶水: 调 Summarize + kwiki.Ingest。
//
// 不引入向量库; 不依赖 UI; 任务/决策由调用方查好后以 meta 值对象传入。
package retro

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/kwiki"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Input is everything Summarize needs, gathered by the caller. The retro
// package does not query the DB itself — it stays a pure transform so it is
// trivially unit-testable.
type Input struct {
	// Project is the project being archived (carries name/status/reason/note).
	Project meta.Project
	// Tasks is the project's full task list (meta.TaskStore.Load result).
	Tasks []meta.Task
}

// TaskStats is the task completion summary of a project.
type TaskStats struct {
	Total     int
	Completed int
	Failed    int
	Cancelled int
	// Open is tasks left unfinished (not in a terminal state) at archive time.
	Open int
	// ByType counts executable issue items per type (task/requirement/bug),
	// excluding discussions (which are tracked separately as decisions).
	ByType map[meta.TaskType]int
}

// Retrospective is the compiled复盘 record: project meta + task stats +
// extracted decisions + derived experience entries.
type Retrospective struct {
	Project meta.Project
	Stats   TaskStats
	// Decisions are the titles of the project's discussion cards
	// (TaskTypeDiscussion) — the project's 决策 trail (#189).
	Decisions []string
	// Experiences are short, human-readable lessons derived from the stats —
	// the seed material the 技能沉淀链 downstream can refine into skills.
	Experiences []string
}

// isTerminal reports whether a task status is a finished state (no longer
// counted as open work).
func isTerminal(s meta.TaskStatus) bool {
	switch s {
	case meta.TaskStatusCompleted, meta.TaskStatusFailed, meta.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// Summarize compiles a retrospective from the project + its tasks. Pure: no IO,
// deterministic output (decisions sorted) so it can be asserted in tests.
func Summarize(in Input) Retrospective {
	r := Retrospective{Project: in.Project}
	r.Stats.ByType = map[meta.TaskType]int{}

	for _, t := range in.Tasks {
		// Discussions are the decision trail, not executable work.
		if t.Type == meta.TaskTypeDiscussion {
			r.Decisions = append(r.Decisions, t.Title)
			continue
		}

		r.Stats.Total++
		typ := t.Type
		if typ == "" {
			typ = meta.TaskTypeTask
		}
		r.Stats.ByType[typ]++

		switch t.Status {
		case meta.TaskStatusCompleted:
			r.Stats.Completed++
		case meta.TaskStatusFailed:
			r.Stats.Failed++
		case meta.TaskStatusCancelled:
			r.Stats.Cancelled++
		}
		if !isTerminal(t.Status) {
			r.Stats.Open++
		}
	}
	sort.Strings(r.Decisions)
	r.Experiences = deriveExperiences(in.Project, r.Stats)
	return r
}

// deriveExperiences turns the stats into short lessons. These are intentionally
// simple, rule-based seeds (无 LLM): the value is fixing the trail into the
// wiki; refining seeds into skills is downstream (#143).
func deriveExperiences(p meta.Project, s TaskStats) []string {
	var out []string
	if p.ArchiveReason == meta.ArchiveReasonSuperseded {
		out = append(out, "项目因竞品出现/大厂已做被砍掉，沉淀其判定无必要继续的依据，避免重复立项。")
	}
	if s.Total > 0 {
		out = append(out, fmt.Sprintf("共 %d 个任务，完成 %d 个（完成率 %d%%）。",
			s.Total, s.Completed, percent(s.Completed, s.Total)))
	}
	if s.Failed > 0 {
		out = append(out, fmt.Sprintf("%d 个任务以失败收场，值得复盘失败模式。", s.Failed))
	}
	if s.Open > 0 {
		out = append(out, fmt.Sprintf("归档时仍有 %d 个未完成任务，确认是否需要迁移到后续项目。", s.Open))
	}
	if len(out) == 0 {
		out = append(out, "无可量化的任务记录；复盘以项目元信息与决策为主。")
	}
	return out
}

func percent(n, total int) int {
	if total == 0 {
		return 0
	}
	return n * 100 / total
}

// Render assembles the retrospective Markdown body that becomes the kwiki page
// content. Deterministic given a Retrospective.
func Render(r Retrospective) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 复盘：%s\n\n", r.Project.Name)

	b.WriteString("## 项目元信息\n\n")
	fmt.Fprintf(&b, "- 状态：%s\n", r.Project.Status)
	if r.Project.ArchiveReason != "" {
		fmt.Fprintf(&b, "- 归档原因：%s\n", r.Project.ArchiveReason)
	}
	if note := strings.TrimSpace(r.Project.ArchiveNote); note != "" {
		fmt.Fprintf(&b, "- 备注：%s\n", note)
	}
	if r.Project.ArchivedAt != nil {
		fmt.Fprintf(&b, "- 归档时间：%s\n", r.Project.ArchivedAt.Format(time.RFC3339))
	}
	b.WriteString("\n")

	b.WriteString("## 任务完成情况\n\n")
	fmt.Fprintf(&b, "- 总计：%d（完成 %d / 失败 %d / 取消 %d / 未完成 %d）\n",
		r.Stats.Total, r.Stats.Completed, r.Stats.Failed, r.Stats.Cancelled, r.Stats.Open)
	for _, typ := range []meta.TaskType{meta.TaskTypeTask, meta.TaskTypeRequirement, meta.TaskTypeBug} {
		if n := r.Stats.ByType[typ]; n > 0 {
			fmt.Fprintf(&b, "  - %s：%d\n", typ, n)
		}
	}
	b.WriteString("\n")

	if len(r.Decisions) > 0 {
		b.WriteString("## 决策记录\n\n")
		for _, d := range r.Decisions {
			fmt.Fprintf(&b, "- %s\n", d)
		}
		b.WriteString("\n")
	}

	b.WriteString("## 经验沉淀\n\n")
	for _, e := range r.Experiences {
		fmt.Fprintf(&b, "- %s\n", e)
	}
	b.WriteString("\n")

	return b.String()
}

// ToInboxItem maps a retrospective into the kwiki ingest value object so it
// lands on the wiki layer. The slug is keyed by project id so re-archiving the
// same project overwrites its retrospective page in place (kwiki.Ingest
// semantics), rather than accumulating duplicates.
func ToInboxItem(r Retrospective) kwiki.InboxItem {
	return kwiki.InboxItem{
		ID:         "retro-" + r.Project.ID,
		Title:      "复盘：" + r.Project.Name,
		Text:       Render(r),
		Source:     "retro",
		Domain:     "work",
		Tags:       []string{"retrospective", "project-archive"},
		CapturedAt: time.Now(),
	}
}
