package meta

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PersonalStore is the Inbox 下游 Task 汇总层 (#67): the lightweight, personal
// landing spot for one-off / 临时 items that should NOT go through the full
// project flow. A personal task has "no project" in the user-facing sense —
// it carries no project_id the rest of the system schedules against.
//
// The tasks table requires a non-empty project_id, so personal tasks live in a
// single reserved project (PersonalProjectID) that is deliberately NOT in the
// workspace registry. The scheduler only sweeps registered workspaces, so this
// bucket is never auto-run — exactly the "轻量、不强制归口" semantics #67 wants.
//
// 立项 (Incubate) is the upgrade gate: a personal task / 需求 worth pursuing
// long-term gets promoted into a real Project (its own workspace_path), keeping
// a backlink to where it came from. No task schema change — promotion is just
// re-homing the row's project_id and re-numbering it in the new project.
type PersonalStore struct {
	tasks *TaskStore
}

// NewPersonalStore returns a PersonalStore sharing the given TaskStore (so the
// per-workspace write lock and #N numbering are reused unchanged).
func NewPersonalStore(tasks *TaskStore) *PersonalStore {
	return &PersonalStore{tasks: tasks}
}

const (
	// PersonalProjectID is the fixed id of the reserved bucket that holds
	// personal (no-project) tasks. It is not a real workspace and never appears
	// in the workspace registry, so the scheduler skips it.
	PersonalProjectID = "__personal__"
	// personalProjectPath is the sentinel workspace_path keying the reserved
	// bucket. The TaskStore is path-keyed; this path is reserved and never maps
	// to a real directory on disk.
	personalProjectPath = "\x00personal"
	// personalProjectName is the bucket's display name.
	personalProjectName = "个人任务"

	// incubatedFromLabel records the personal-task origin on a project promoted
	// via 立项, so the trail "this project grew out of that personal task"
	// survives. Format: "incubated-from:<personalTaskID>".
	incubatedFromLabel = "incubated-from:"

	// capturedFromLabel records the originating inbox_item id on a personal task
	// captured out of the Inbox (Inbox #60 items have no task row to Links-target).
	// Format: "captured-from:<inboxItemID>".
	capturedFromLabel = "captured-from:"
)

// ensurePersonalProject creates the reserved personal bucket row if missing,
// pinned to the fixed id + sentinel path. Idempotent.
func (s *PersonalStore) ensurePersonalProject() error {
	var existing string
	err := s.tasks.db.sql.QueryRow(
		`SELECT id FROM projects WHERE id = ?`, PersonalProjectID).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	now := timeToStr(time.Now().UTC())
	_, err = s.tasks.db.sql.Exec(`
		INSERT INTO projects (id, name, workspace_path, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		PersonalProjectID, personalProjectName, personalProjectPath, now, now)
	return err
}

// List returns the personal tasks (the reserved bucket), oldest-first like the
// regular task list. Personal tasks are exactly those with no real project.
func (s *PersonalStore) List() ([]Task, error) {
	if err := s.ensurePersonalProject(); err != nil {
		return nil, err
	}
	cfg, err := s.tasks.Load(personalProjectPath)
	if err != nil {
		return nil, err
	}
	return cfg.Tasks, nil
}

// Capture lands a new lightweight personal task. Title is required. fromInbox,
// when set, records the originating inbox_item id as a label backlink. Returns
// the stored task (with id + #N assigned).
func (s *PersonalStore) Capture(title, description, fromInbox string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, fmt.Errorf("meta: personal task title is required")
	}
	if err := s.ensurePersonalProject(); err != nil {
		return Task{}, err
	}
	now := time.Now().UTC()
	t := Task{
		ID:           newID(),
		Title:        title,
		Description:  description,
		Type:         TaskTypeTask,
		IssueState:   IssueOpen,
		Status:       TaskStatusPending,
		ScheduleType: ScheduleTypeImmediate,
		CreatedAt:    now,
		UpdatedAt:    now,
		Replies:      []Reply{},
		Sessions:     []SessionMetadata{},
	}
	if ref := strings.TrimSpace(fromInbox); ref != "" {
		t.Labels = append(t.Labels, capturedFromLabel+ref)
	}
	if err := s.tasks.Mutate(personalProjectPath, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, t)
		return true
	}); err != nil {
		return Task{}, err
	}
	saved, _, err := s.tasks.GetTask(t.ID)
	return saved, err
}

// IncubateResult is what Incubate returns: the freshly-created project and the
// task as it now lives inside that project.
type IncubateResult struct {
	Project Project `json:"project"`
	Task    Task    `json:"task"`
}

// Incubate is the 立项 gate (#67 Phase C): promote a personal task into a new
// long-term Project. It creates the project (name + workspacePath), re-homes the
// task into it (new project_id, re-numbered #N), and stamps an incubated-from
// backlink label. workspacePath must be unique — a project already registered at
// that path is an error (we don't silently merge into someone else's project).
//
// Optional milestones seed the project's roadmap (#67: 目标/里程碑/Roadmap). They
// are created in order through the existing milestone store.
func (s *PersonalStore) Incubate(personalTaskID, projectName, workspacePath string, milestones []string) (IncubateResult, error) {
	projectName = strings.TrimSpace(projectName)
	workspacePath = strings.TrimSpace(workspacePath)
	if projectName == "" || workspacePath == "" {
		return IncubateResult{}, fmt.Errorf("meta: project name and workspacePath are required")
	}
	if workspacePath == personalProjectPath {
		return IncubateResult{}, fmt.Errorf("meta: workspacePath is reserved")
	}

	// The task must exist and currently live in the personal bucket.
	task, ok, err := s.tasks.GetTask(personalTaskID)
	if err != nil {
		return IncubateResult{}, err
	}
	if !ok {
		return IncubateResult{}, ErrNotFound
	}
	if task.WorkspacePath != personalProjectPath {
		return IncubateResult{}, fmt.Errorf("meta: task %s is not a personal task", personalTaskID)
	}

	// Reject promotion into an existing project path — incubation always opens a
	// fresh project.
	existing, err := s.tasks.db.projectIDByPath(workspacePath)
	if err != nil {
		return IncubateResult{}, err
	}
	if existing != "" {
		return IncubateResult{}, fmt.Errorf("meta: a project already exists at %s", workspacePath)
	}

	// Create the project AS a workspace (unified registry): it now carries the
	// sidebar/terminal/chat fields, so 立项 makes it appear in the sidebar and
	// global task board automatically — no separate workspace registration. The
	// cc-connect bridge + guide files are wired by the handler's onIncubated hook.
	projectID := newID()
	if err := s.tasks.db.EnsureWorkspaceProject(Project{
		ID:            projectID,
		Name:          projectName,
		WorkspacePath: workspacePath,
		DefaultAgent:  "claudecode",
	}); err != nil {
		return IncubateResult{}, err
	}

	// Re-home the row: switch project_id and clear the per-project number so the
	// next step re-assigns it within the new project.
	now := timeToStr(time.Now().UTC())
	if _, err := s.tasks.db.sql.Exec(`
		UPDATE tasks SET project_id = ?, number = 0, updated_at = ? WHERE id = ?`,
		projectID, now, personalTaskID); err != nil {
		return IncubateResult{}, err
	}

	// Re-number inside the new project (mirrors upsertTaskTx's MAX+1 assignment).
	if err := s.assignNumber(projectID, personalTaskID); err != nil {
		return IncubateResult{}, err
	}

	// Stamp the incubated-from backlink label idempotently.
	if err := s.addLabel(personalTaskID, incubatedFromLabel+personalTaskID); err != nil {
		return IncubateResult{}, err
	}

	// Seed roadmap milestones (in order; skip blanks; dedupe by name).
	for _, name := range milestones {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := s.tasks.CreateMilestone(workspacePath, name, "", nil, ""); err != nil {
			// Duplicate names are not fatal — the roadmap just dedupes.
			if err != ErrMilestoneExists {
				return IncubateResult{}, err
			}
		}
	}

	project, _, err := s.tasks.db.GetProject(projectID)
	if err != nil {
		return IncubateResult{}, err
	}
	promoted, _, err := s.tasks.GetTask(personalTaskID)
	if err != nil {
		return IncubateResult{}, err
	}
	return IncubateResult{Project: project, Task: promoted}, nil
}

// assignNumber gives the task the next free #N within its (new) project.
func (s *PersonalStore) assignNumber(projectID, taskID string) error {
	var n int
	if err := s.tasks.db.sql.QueryRow(
		`SELECT COALESCE(MAX(number), 0) + 1 FROM tasks WHERE project_id = ?`,
		projectID).Scan(&n); err != nil {
		return err
	}
	_, err := s.tasks.db.sql.Exec(`UPDATE tasks SET number = ? WHERE id = ?`, n, taskID)
	return err
}

// addLabel appends label to a task's labels JSON unless already present.
func (s *PersonalStore) addLabel(taskID, label string) error {
	var raw string
	if err := s.tasks.db.sql.QueryRow(
		`SELECT labels FROM tasks WHERE id = ?`, taskID).Scan(&raw); err != nil {
		return err
	}
	labels := jsonToStrings(raw)
	for _, l := range labels {
		if l == label {
			return nil
		}
	}
	labels = append(labels, label)
	_, err := s.tasks.db.sql.Exec(`UPDATE tasks SET labels = ? WHERE id = ?`,
		stringsToJSON(labels), taskID)
	return err
}
