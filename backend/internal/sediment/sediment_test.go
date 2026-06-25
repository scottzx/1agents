package sediment

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/retro"
)

func sampleRetro() retro.Retrospective {
	at := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	return retro.Summarize(retro.Input{
		Project: meta.Project{
			ID:            "p1",
			Name:          "Remote Agent",
			Status:        meta.ProjectStatusArchived,
			ArchiveReason: meta.ArchiveReasonSuperseded,
			ArchivedAt:    &at,
		},
		Tasks: []meta.Task{
			{Title: "建终端", Type: meta.TaskTypeTask, Status: meta.TaskStatusCompleted},
			{Title: "建文件管理", Type: meta.TaskTypeTask, Status: meta.TaskStatusFailed},
			{Title: "未完成的活", Type: meta.TaskTypeTask, Status: meta.TaskStatusPending},
		},
	})
}

func TestDefaultChainLayersAndEdges(t *testing.T) {
	c := DefaultChain()
	want := []Layer{LayerExperience, LayerSkill, LayerTaskTemplate, LayerProjectTemplate}
	if got := c.Layers(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Layers() = %v, want %v", got, want)
	}
	if nxt, ok := c.Next(LayerExperience); !ok || nxt != LayerSkill {
		t.Errorf("Next(Experience) = %v,%v want skill,true", nxt, ok)
	}
	if _, ok := c.Next(LayerProjectTemplate); ok {
		t.Errorf("Next(ProjectTemplate) should be terminal (no edge)")
	}
}

func TestPromoteExperiencesFiltersStatsAndKeepsMethods(t *testing.T) {
	r := sampleRetro()
	cands := PromoteExperiences(r)
	if len(cands) == 0 {
		t.Fatal("expected at least one skill candidate")
	}
	for _, c := range cands {
		if c.Layer != LayerSkill {
			t.Errorf("candidate layer = %q, want skill", c.Layer)
		}
		if strings.Contains(c.Description, "完成率") {
			t.Errorf("stat line should have been filtered: %q", c.Description)
		}
		if len(c.SourceIDs) != 1 || c.SourceIDs[0] != "retro-p1" {
			t.Errorf("SourceIDs = %v, want [retro-p1]", c.SourceIDs)
		}
		if !strings.HasPrefix(c.Title, "Remote Agent · ") {
			t.Errorf("title not project-scoped: %q", c.Title)
		}
	}
	// The superseded-reason lesson is a method, so it must survive the filter.
	var found bool
	for _, c := range cands {
		if strings.Contains(c.Description, "竞品") {
			found = true
		}
	}
	if !found {
		t.Error("expected the superseded-reason lesson to be promoted")
	}
}

func TestPromoteSkillsGroupsByNamespace(t *testing.T) {
	skills := []agent.Skill{
		{Name: "code-review"},
		{Name: "code-format"},
		{Name: "deep-research"},
		{Name: ""}, // ignored
	}
	tts := PromoteSkills(skills)
	if len(tts) != 2 {
		t.Fatalf("got %d task templates, want 2 (code, deep)", len(tts))
	}
	// Sorted by namespace key: "code" before "deep".
	if tts[0].Title != "code 任务模板" {
		t.Errorf("first template title = %q", tts[0].Title)
	}
	if !reflect.DeepEqual(tts[0].Skills, []string{"code-format", "code-review"}) {
		t.Errorf("code template skills = %v", tts[0].Skills)
	}
	if tts[0].Layer != LayerTaskTemplate {
		t.Errorf("layer = %q, want task_template", tts[0].Layer)
	}
	if len(tts[0].Checks) == 0 {
		t.Error("expected a default check item")
	}
}

func TestPromoteTaskTemplatesComposesProject(t *testing.T) {
	tts := []TaskTemplateCandidate{
		{
			Candidate: Candidate{Title: "A 任务模板", SourceIDs: []string{"skill-a"}},
			Skills:    []string{"a-one", "shared"},
			Checks:    []string{"chk-common", "chk-a"},
		},
		{
			Candidate: Candidate{Title: "B 任务模板", SourceIDs: []string{"skill-b"}},
			Skills:    []string{"b-one", "shared"},
			Checks:    []string{"chk-common", "chk-b"},
		},
	}
	pc := PromoteTaskTemplates("Demo", tts)
	if pc.Layer != LayerProjectTemplate {
		t.Errorf("layer = %q", pc.Layer)
	}
	if pc.Title != "Demo 项目模板" {
		t.Errorf("title = %q", pc.Title)
	}
	if !reflect.DeepEqual(pc.TaskOrder, []string{"A 任务模板", "B 任务模板"}) {
		t.Errorf("TaskOrder = %v", pc.TaskOrder)
	}
	// Skills deduped (shared appears once) and sorted.
	if !reflect.DeepEqual(pc.Skills, []string{"a-one", "b-one", "shared"}) {
		t.Errorf("Skills = %v", pc.Skills)
	}
	// Checks deduped, input order preserved.
	if !reflect.DeepEqual(pc.Checks, []string{"chk-common", "chk-a", "chk-b"}) {
		t.Errorf("Checks = %v", pc.Checks)
	}
	if !reflect.DeepEqual(pc.SourceIDs, []string{"skill-a", "skill-b"}) {
		t.Errorf("SourceIDs = %v", pc.SourceIDs)
	}
}

func TestRunEndToEnd(t *testing.T) {
	res := Run(sampleRetro(), []agent.Skill{
		{Name: "code-review"},
		{Name: "code-format"},
	})
	if len(res.Skills) == 0 {
		t.Error("expected skill candidates from retro experiences")
	}
	if len(res.TaskTemplates) != 1 {
		t.Errorf("expected 1 task template (code namespace), got %d", len(res.TaskTemplates))
	}
	if res.ProjectTemplate.Title != "Remote Agent 项目模板" {
		t.Errorf("project template title = %q", res.ProjectTemplate.Title)
	}
	if len(res.ProjectTemplate.Skills) == 0 {
		t.Error("project template should carry the union of skills")
	}
}
