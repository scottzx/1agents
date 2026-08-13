package execution

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

type Repository struct{ db *meta.DB }

func NewRepository(db *meta.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("execution: nil database")
	}
	r := &Repository{db: db}
	if err := r.ensureSchema(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Repository) ensureSchema() error {
	_, err := r.db.SQL().Exec(`
		CREATE TABLE IF NOT EXISTS kernel_execution_jobs (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL, work_item_id TEXT NOT NULL,
			business_ref TEXT NOT NULL DEFAULT '', executor_kind TEXT NOT NULL,
			profile_id TEXT NOT NULL DEFAULT '', profile_source TEXT NOT NULL DEFAULT '',
			legacy_agent_type TEXT NOT NULL DEFAULT '', function_type TEXT NOT NULL DEFAULT '',
			preamble_function_type TEXT NOT NULL DEFAULT '',
			cwd TEXT NOT NULL DEFAULT '', capabilities_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL, revision INTEGER NOT NULL, timeout_minutes INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 1, blocked_code TEXT NOT NULL DEFAULT '',
			blocked_reason TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			UNIQUE(project_id, work_item_id)
		);
		CREATE TABLE IF NOT EXISTS kernel_execution_triggers (
			id TEXT PRIMARY KEY, job_id TEXT NOT NULL UNIQUE, kind TEXT NOT NULL,
			spec_json TEXT NOT NULL, timezone TEXT NOT NULL DEFAULT '',
			misfire_policy TEXT NOT NULL DEFAULT 'skip', overlap_policy TEXT NOT NULL DEFAULT 'forbid',
			status TEXT NOT NULL, next_run_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_kernel_execution_jobs_project ON kernel_execution_jobs(project_id, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_kernel_execution_triggers_due ON kernel_execution_triggers(status, next_run_at);
	`)
	if err != nil {
		return err
	}
	return r.ensureJobColumns()
}

func (r *Repository) ensureJobColumns() error {
	rows, err := r.db.SQL().Query(`PRAGMA table_info(kernel_execution_jobs)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if have["preamble_function_type"] {
		return nil
	}
	_, err = r.db.SQL().Exec(`ALTER TABLE kernel_execution_jobs ADD COLUMN preamble_function_type TEXT NOT NULL DEFAULT ''`)
	return err
}

func (r *Repository) Create(job Job) (Job, error) {
	now := time.Now().UTC()
	job.ID = meta.NewID()
	job.Revision = 1
	job.CreatedAt, job.UpdatedAt = now, now
	caps, _ := json.Marshal(job.Capabilities)
	_, err := r.db.SQL().Exec(`INSERT INTO kernel_execution_jobs (
		id, project_id, work_item_id, business_ref, executor_kind, profile_id, profile_source,
		legacy_agent_type, function_type, preamble_function_type, cwd, capabilities_json, status, revision,
		timeout_minutes, max_attempts, blocked_code, blocked_reason, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, ?)`,
		job.ID, job.ProjectID, job.WorkItemID, job.BusinessRef, job.ExecutorKind,
		job.ProfileID, job.ProfileSource, job.LegacyAgentType, job.FunctionType, job.PreambleFunctionType, job.Cwd,
		string(caps), job.Status, job.Revision, job.TimeoutMinutes, job.MaxAttempts,
		formatTime(now), formatTime(now))
	if err != nil {
		return Job{}, err
	}
	return job, nil
}

func (r *Repository) Get(id string) (Job, bool, error) {
	return scanJob(r.db.SQL().QueryRow(jobColumns+` WHERE id = ?`, id))
}

func (r *Repository) ListByProject(projectID string) ([]Job, error) {
	query := jobColumns + ` ORDER BY updated_at DESC, id`
	args := []any{}
	if projectID != "" {
		query = jobColumns + ` WHERE project_id = ? ORDER BY updated_at DESC, id`
		args = append(args, projectID)
	}
	rows, err := r.db.SQL().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []Job{}
	for rows.Next() {
		job, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Repository) Update(job Job) (Job, error) {
	caps, _ := json.Marshal(job.Capabilities)
	job.UpdatedAt = time.Now().UTC()
	result, err := r.db.SQL().Exec(`UPDATE kernel_execution_jobs SET
		profile_id=?, profile_source=?, legacy_agent_type=?, function_type=?, preamble_function_type=?, cwd=?,
		capabilities_json=?, status=?, revision=?, timeout_minutes=?, max_attempts=?,
		blocked_code=?, blocked_reason=?, updated_at=? WHERE id=?`,
		job.ProfileID, job.ProfileSource, job.LegacyAgentType, job.FunctionType, job.PreambleFunctionType, job.Cwd,
		string(caps), job.Status, job.Revision, job.TimeoutMinutes, job.MaxAttempts,
		job.BlockedCode, job.BlockedReason, formatTime(job.UpdatedAt), job.ID)
	if err != nil {
		return Job{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Job{}, meta.ErrNotFound
	}
	return job, nil
}

func (r *Repository) SetStatus(id, status string) error {
	result, err := r.db.SQL().Exec(`UPDATE kernel_execution_jobs SET status=?, updated_at=? WHERE id=?`, status, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return meta.ErrNotFound
	}
	return nil
}

func (r *Repository) UpsertTrigger(trigger Trigger) (Trigger, error) {
	now := time.Now().UTC()
	if trigger.ID == "" {
		trigger.ID = meta.NewID()
	}
	trigger.CreatedAt, trigger.UpdatedAt = now, now
	_, err := r.db.SQL().Exec(`INSERT INTO kernel_execution_triggers (
		id, job_id, kind, spec_json, timezone, misfire_policy, overlap_policy, status, next_run_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(job_id) DO UPDATE SET kind=excluded.kind, spec_json=excluded.spec_json,
		timezone=excluded.timezone, misfire_policy=excluded.misfire_policy, overlap_policy=excluded.overlap_policy,
		status=excluded.status, next_run_at=excluded.next_run_at, updated_at=excluded.updated_at`,
		trigger.ID, trigger.JobID, trigger.Kind, string(trigger.Spec), trigger.Timezone,
		trigger.MisfirePolicy, trigger.OverlapPolicy, trigger.Status, nullableTime(trigger.NextRunAt), formatTime(now), formatTime(now))
	if err != nil {
		return Trigger{}, err
	}
	return r.TriggerByJob(trigger.JobID)
}

func (r *Repository) TriggerByJob(jobID string) (Trigger, error) {
	var t Trigger
	var spec, next, created, updated string
	err := r.db.SQL().QueryRow(`SELECT id, job_id, kind, spec_json, timezone, misfire_policy, overlap_policy, status,
		COALESCE(next_run_at, ''), created_at, updated_at FROM kernel_execution_triggers WHERE job_id=?`, jobID).
		Scan(&t.ID, &t.JobID, &t.Kind, &spec, &t.Timezone, &t.MisfirePolicy, &t.OverlapPolicy, &t.Status, &next, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Trigger{}, meta.ErrNotFound
	}
	if err != nil {
		return Trigger{}, err
	}
	t.Spec = json.RawMessage(spec)
	t.NextRunAt = parseTimePtr(next)
	t.CreatedAt = parseTime(created)
	t.UpdatedAt = parseTime(updated)
	return t, nil
}

func (r *Repository) DeleteTrigger(jobID string) error {
	result, err := r.db.SQL().Exec(`DELETE FROM kernel_execution_triggers WHERE job_id=?`, jobID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return meta.ErrNotFound
	}
	return nil
}

func (r *Repository) DueTriggers(now time.Time) ([]Trigger, error) {
	rows, err := r.db.SQL().Query(`SELECT id, job_id, kind, spec_json, timezone, misfire_policy, overlap_policy, status,
		COALESCE(next_run_at, ''), created_at, updated_at FROM kernel_execution_triggers
		WHERE status='armed' AND next_run_at IS NOT NULL AND next_run_at <= ? ORDER BY next_run_at, id`, formatTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trigger
	for rows.Next() {
		var t Trigger
		var spec, next, created, updated string
		if err := rows.Scan(&t.ID, &t.JobID, &t.Kind, &spec, &t.Timezone, &t.MisfirePolicy, &t.OverlapPolicy, &t.Status, &next, &created, &updated); err != nil {
			return nil, err
		}
		t.Spec, t.NextRunAt, t.CreatedAt, t.UpdatedAt = json.RawMessage(spec), parseTimePtr(next), parseTime(created), parseTime(updated)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) AdvanceTrigger(id, status string, next *time.Time) error {
	result, err := r.db.SQL().Exec(`UPDATE kernel_execution_triggers SET status=?, next_run_at=?, updated_at=? WHERE id=?`, status, nullableTime(next), formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return meta.ErrNotFound
	}
	return nil
}

const jobColumns = `SELECT id, project_id, work_item_id, business_ref, executor_kind, profile_id, profile_source,
	legacy_agent_type, function_type, preamble_function_type, cwd, capabilities_json, status, revision, timeout_minutes,
	max_attempts, blocked_code, blocked_reason, created_at, updated_at FROM kernel_execution_jobs`

func scanJob(row *sql.Row) (Job, bool, error) {
	var job Job
	var caps, created, updated string
	err := row.Scan(&job.ID, &job.ProjectID, &job.WorkItemID, &job.BusinessRef, &job.ExecutorKind, &job.ProfileID, &job.ProfileSource,
		&job.LegacyAgentType, &job.FunctionType, &job.PreambleFunctionType, &job.Cwd, &caps, &job.Status, &job.Revision, &job.TimeoutMinutes, &job.MaxAttempts,
		&job.BlockedCode, &job.BlockedReason, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	_ = json.Unmarshal([]byte(caps), &job.Capabilities)
	job.CreatedAt = parseTime(created)
	job.UpdatedAt = parseTime(updated)
	return job, true, nil
}

func scanJobRows(rows *sql.Rows) (Job, error) {
	var job Job
	var caps, created, updated string
	if err := rows.Scan(&job.ID, &job.ProjectID, &job.WorkItemID, &job.BusinessRef, &job.ExecutorKind, &job.ProfileID, &job.ProfileSource,
		&job.LegacyAgentType, &job.FunctionType, &job.PreambleFunctionType, &job.Cwd, &caps, &job.Status, &job.Revision, &job.TimeoutMinutes, &job.MaxAttempts,
		&job.BlockedCode, &job.BlockedReason, &created, &updated); err != nil {
		return Job{}, err
	}
	_ = json.Unmarshal([]byte(caps), &job.Capabilities)
	job.CreatedAt = parseTime(created)
	job.UpdatedAt = parseTime(updated)
	return job, nil
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func parseTime(s string) time.Time  { t, _ := time.Parse(time.RFC3339Nano, s); return t.UTC() }
func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t := parseTime(s)
	return &t
}
func nullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return formatTime(*t)
}

func (r *Repository) ValidateWorkItem(projectID, workItemID string) error {
	var typ string
	err := r.db.SQL().QueryRow(`SELECT type FROM project_items WHERE id=? AND project_id=?`, workItemID, projectID).Scan(&typ)
	if errors.Is(err, sql.ErrNoRows) {
		return meta.ErrNotFound
	}
	if err != nil {
		return err
	}
	if typ != "" && typ != "task" {
		return fmt.Errorf("execution: work item %s is not executable", workItemID)
	}
	return nil
}

func (r *Repository) ProjectDefaultProfile(projectID string) (string, error) {
	var profileID string
	err := r.db.SQL().QueryRow(`SELECT default_profile_id FROM projects WHERE id=?`, projectID).Scan(&profileID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", meta.ErrNotFound
	}
	return profileID, err
}
