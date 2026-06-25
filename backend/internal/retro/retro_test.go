package retro

import (
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/kwiki"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func sampleInput() Input {
	at := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	return Input{
		Project: meta.Project{
			ID:            "p1",
			Name:          "Remote Agent",
			Status:        meta.ProjectStatusArchived,
			ArchiveReason: meta.ArchiveReasonCompleted,
			ArchiveNote:   "阶段一收尾",
			ArchivedAt:    &at,
		},
		Tasks: []meta.Task{
			{Title: "建终端", Type: meta.TaskTypeTask, Status: meta.TaskStatusCompleted},
			{Title: "建文件管理", Type: meta.TaskTypeTask, Status: meta.TaskStatusFailed},
			{Title: "要支持多端", Type: meta.TaskTypeRequirement, Status: meta.TaskStatusPending},
			{Title: "崩溃修复", Type: meta.TaskTypeBug, Status: meta.TaskStatusCompleted},
			{Title: "要不要做向量库", Type: meta.TaskTypeDiscussion},
			{Title: "是否合并 happy", Type: meta.TaskTypeDiscussion},
			{Title: "无类型任务", Type: "", Status: meta.TaskStatusCancelled},
		},
	}
}

func TestSummarizeStats(t *testing.T) {
	r := Summarize(sampleInput())

	// 5 executable items (2 task + 1 req + 1 bug + 1 untyped→task), 2 discussions excluded.
	if r.Stats.Total != 5 {
		t.Errorf("Total = %d, want 5", r.Stats.Total)
	}
	if r.Stats.Completed != 2 {
		t.Errorf("Completed = %d, want 2", r.Stats.Completed)
	}
	if r.Stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", r.Stats.Failed)
	}
	if r.Stats.Cancelled != 1 {
		t.Errorf("Cancelled = %d, want 1", r.Stats.Cancelled)
	}
	// Open = non-terminal: only the pending requirement.
	if r.Stats.Open != 1 {
		t.Errorf("Open = %d, want 1", r.Stats.Open)
	}
	// Untyped task folds into TaskTypeTask: 2 explicit + 1 untyped = 3.
	if got := r.Stats.ByType[meta.TaskTypeTask]; got != 3 {
		t.Errorf("ByType[task] = %d, want 3", got)
	}
	if got := r.Stats.ByType[meta.TaskTypeRequirement]; got != 1 {
		t.Errorf("ByType[requirement] = %d, want 1", got)
	}
	if got := r.Stats.ByType[meta.TaskTypeBug]; got != 1 {
		t.Errorf("ByType[bug] = %d, want 1", got)
	}
}

func TestSummarizeDecisionsSorted(t *testing.T) {
	r := Summarize(sampleInput())
	want := []string{"是否合并 happy", "要不要做向量库"}
	if len(r.Decisions) != len(want) {
		t.Fatalf("Decisions = %v, want %v", r.Decisions, want)
	}
	for i := range want {
		if r.Decisions[i] != want[i] {
			t.Errorf("Decisions[%d] = %q, want %q", i, r.Decisions[i], want[i])
		}
	}
}

func TestDeriveExperiencesSuperseded(t *testing.T) {
	in := sampleInput()
	in.Project.Status = meta.ProjectStatusKilled
	in.Project.ArchiveReason = meta.ArchiveReasonSuperseded
	r := Summarize(in)
	joined := strings.Join(r.Experiences, "\n")
	if !strings.Contains(joined, "竞品") {
		t.Errorf("expected superseded lesson in experiences, got %v", r.Experiences)
	}
}

func TestDeriveExperiencesEmpty(t *testing.T) {
	r := Summarize(Input{Project: meta.Project{ID: "x", Name: "Empty"}})
	if len(r.Experiences) != 1 {
		t.Fatalf("want one fallback experience, got %v", r.Experiences)
	}
	if r.Stats.Total != 0 {
		t.Errorf("Total = %d, want 0", r.Stats.Total)
	}
}

func TestRenderContainsSections(t *testing.T) {
	body := Render(Summarize(sampleInput()))
	for _, want := range []string{
		"# 复盘：Remote Agent",
		"## 项目元信息",
		"归档原因：completed",
		"阶段一收尾",
		"## 任务完成情况",
		"## 决策记录",
		"是否合并 happy",
		"## 经验沉淀",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q\n---\n%s", want, body)
		}
	}
}

func TestToInboxItem(t *testing.T) {
	item := ToInboxItem(Summarize(sampleInput()))
	if item.ID != "retro-p1" {
		t.Errorf("ID = %q, want retro-p1", item.ID)
	}
	if item.Source != "retro" {
		t.Errorf("Source = %q, want retro", item.Source)
	}
	if !strings.Contains(item.Text, "# 复盘：Remote Agent") {
		t.Errorf("Text missing rendered body: %q", item.Text)
	}
}

func TestArchiveIngestsToKwiki(t *testing.T) {
	store, err := kwiki.Open(t.TempDir())
	if err != nil {
		t.Fatalf("kwiki.Open: %v", err)
	}
	page, err := Archive(store, sampleInput())
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if page.Slug == "" {
		t.Fatal("expected non-empty slug")
	}
	if !strings.Contains(page.Body, "任务完成情况") {
		t.Errorf("ingested page body missing retro content: %q", page.Body)
	}
	// Re-archiving overwrites in place (same slug), no duplicate.
	page2, err := Archive(store, sampleInput())
	if err != nil {
		t.Fatalf("re-Archive: %v", err)
	}
	if page2.Slug != page.Slug {
		t.Errorf("slug changed on re-archive: %q vs %q", page2.Slug, page.Slug)
	}
}

func TestArchiveNilStore(t *testing.T) {
	if _, err := Archive(nil, sampleInput()); err == nil {
		t.Fatal("expected error for nil store")
	}
}
