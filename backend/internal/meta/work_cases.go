package meta

// WorkCase is the kernel-owned long-running business matter frozen by
// docs/architecture/enterprise-foundation-v1.0.0.md §4.3 (D2): it sits between
// domain objects (referenced through stable DomainRefs, never embedded) and
// executable Tasks, aggregating objective, participants, phase projection,
// subject refs and the task/session/artifact/event association spine.
//
// Kernel neutrality (§3.1): the WorkCase schema carries NO domain-specific
// fields — no opportunity_stage, customer_budget, sku, listing_status, etc.
// CaseDefinition names the application-defined case type; CurrentPhase is an
// opaque application projection the kernel stores but never interprets.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/scottzx/1Agents/backend/internal/domainref"
)

// CaseStatus is the generic WorkCase lifecycle state (§4.3). The lifecycle is
// domain-neutral by design: open/suspended/closed/cancelled express
// coordination progress, never a presales or commerce stage.
type CaseStatus string

const (
	CaseStatusOpen      CaseStatus = "open"
	CaseStatusSuspended CaseStatus = "suspended"
	CaseStatusClosed    CaseStatus = "closed"
	CaseStatusCancelled CaseStatus = "cancelled"
)

// Valid reports whether s is one of the four lifecycle states.
func (s CaseStatus) Valid() bool {
	switch s {
	case CaseStatusOpen, CaseStatusSuspended, CaseStatusClosed, CaseStatusCancelled:
		return true
	}
	return false
}

// Terminal reports whether s is a final state. Terminal cases never move again:
// every transition out of a terminal state is rejected (acceptance: 非法终态回退被拒绝).
func (s CaseStatus) Terminal() bool {
	return s == CaseStatusClosed || s == CaseStatusCancelled
}

// caseTransitions is the full legal-transition matrix. Anything not listed is
// rejected, including every move out of a terminal state and same-state no-ops.
var caseTransitions = map[CaseStatus][]CaseStatus{
	CaseStatusOpen:      {CaseStatusSuspended, CaseStatusClosed, CaseStatusCancelled},
	CaseStatusSuspended: {CaseStatusOpen, CaseStatusClosed, CaseStatusCancelled},
}

// CanTransition reports whether moving from→to is a legal lifecycle step.
func CanTransition(from, to CaseStatus) bool {
	for _, next := range caseTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// CaseLinkKind enumerates the kernel-known association kinds between a
// WorkCase and other runtime objects (§4.3: Task、Session、Artifact 关联).
type CaseLinkKind string

const (
	CaseLinkTask     CaseLinkKind = "task"
	CaseLinkSession  CaseLinkKind = "session"
	CaseLinkArtifact CaseLinkKind = "artifact"
)

// Valid reports whether k is a known link kind.
func (k CaseLinkKind) Valid() bool {
	switch k {
	case CaseLinkTask, CaseLinkSession, CaseLinkArtifact:
		return true
	}
	return false
}

var (
	// ErrInvalidCaseTransition rejects any lifecycle move outside the legal
	// matrix — most importantly every regression out of a terminal state.
	ErrInvalidCaseTransition = errors.New("meta: invalid work case transition")
	// ErrCaseVersionConflict is the optimistic-concurrency rejection: the
	// caller's expectedVersion did not match the stored version.
	ErrCaseVersionConflict = errors.New("meta: work case version conflict")
)

// CaseParticipant is one actor collaborating on a WorkCase (§4.3 参与者). The
// kernel only records identity and an application-defined role key; it never
// interprets Role.
type CaseParticipant struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // e.g. "user" | "agent" — kernel stores, does not police
	Name string `json:"name,omitempty"`
	Role string `json:"role,omitempty"` // application-defined role key
}

// WorkCase is one persisted long-running business matter.
type WorkCase struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"` // == projects.id
	Title       string `json:"title"`
	Objective   string `json:"objective,omitempty"`
	// CaseDefinition names the application-defined case type whose rules
	// interpret this case (业务阶段键由 CaseDefinition 解释, §4.1). Opaque to the kernel.
	CaseDefinition string     `json:"caseDefinition,omitempty"`
	Status         CaseStatus `json:"status"`
	// Owner is the current responsible person; CreatedBy the originator (§4.3).
	Owner     string `json:"owner,omitempty"`
	CreatedBy string `json:"createdBy,omitempty"`
	// CurrentPhase is an application projection only: the kernel stores and
	// returns it verbatim and never validates it against any stage vocabulary
	// (task #322: current_phase 仅为应用投影，不在内核编码售前或电商规则).
	CurrentPhase string `json:"currentPhase,omitempty"`
	// PrimarySubject is the main DomainRef (canonical string form); SubjectRefs
	// holds zero or more related DomainRefs (§4.3: 一个主 DomainRef 和零到多个相关
	// DomainRef). The kernel validates ref structure but never resolves them —
	// resolution goes through the owning domain's QueryProvider (§4.2).
	PrimarySubject string            `json:"primarySubject,omitempty"`
	SubjectRefs    []string          `json:"subjectRefs,omitempty"`
	Participants   []CaseParticipant `json:"participants,omitempty"`
	// ExpectedCloseAt is the SLA / 期望时间 horizon (§4.3). Nil when unset.
	ExpectedCloseAt *time.Time `json:"expectedCloseAt,omitempty"`
	// Version is the optimistic-concurrency token: 1 on create, bumped by every
	// successful mutation. Mutators must pass the version they read.
	Version     int        `json:"version"`
	CloseReason string     `json:"closeReason,omitempty"`
	ClosedAt    *time.Time `json:"closedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// Ref returns the versioned CaseRef (§4.3 / domainref) identifying this case.
func (c WorkCase) Ref() (domainref.CaseRef, error) {
	return domainref.NewCaseRef(c.WorkspaceID, c.ID, 0)
}

// CaseLink is one association edge between a WorkCase and a runtime object.
type CaseLink struct {
	CaseID    string       `json:"caseId"`
	Kind      CaseLinkKind `json:"kind"`
	TargetID  string       `json:"targetId"`
	CreatedAt time.Time    `json:"createdAt"`
}

// WorkCasePatch carries partial edits for WorkCaseStore.Update. Nil fields are
// left untouched.
//
// Deliberately absent: Status and CurrentPhase. Lifecycle moves go through
// Transition, and phase is an application projection that may only advance
// through a registered Command (SetPhaseInTx via the command gateway, #323:
// 禁止直接修改 Case phase) — never through a generic patch.
type WorkCasePatch struct {
	Title           *string
	Objective       *string
	CaseDefinition  *string
	Owner           *string
	PrimarySubject  *string
	SubjectRefs     *[]string
	Participants    *[]CaseParticipant
	ExpectedCloseAt *time.Time
}

// WorkCaseStore is the kernel's SQLite-backed WorkCase persistence (§4.3, §7.1:
// new kernel tables, written only by this store).
type WorkCaseStore struct {
	db *DB
}

// NewWorkCaseStore returns a WorkCaseStore over db.
func NewWorkCaseStore(db *DB) *WorkCaseStore {
	return &WorkCaseStore{db: db}
}

// ProjectIDForPath resolves the project id for a workspace path ("" when the
// workspace is unknown). Mirrors TaskStore.ProjectIDForPath.
func (s *WorkCaseStore) ProjectIDForPath(workspacePath string) (string, error) {
	return s.db.projectIDByPath(workspacePath)
}

const workCaseCols = `id, project_id, case_definition, status, title, objective,
	owner, created_by, current_phase, primary_subject, subject_refs, participants,
	expected_close_at, version, close_reason, closed_at, created_at, updated_at`

func scanWorkCase(r rowScanner) (WorkCase, error) {
	var c WorkCase
	var status string
	var subjectRefs, participants string
	var expectedCloseAt, closedAt sql.NullString
	var createdAt, updatedAt string
	if err := r.Scan(&c.ID, &c.WorkspaceID, &c.CaseDefinition, &status, &c.Title,
		&c.Objective, &c.Owner, &c.CreatedBy, &c.CurrentPhase, &c.PrimarySubject,
		&subjectRefs, &participants, &expectedCloseAt, &c.Version, &c.CloseReason,
		&closedAt, &createdAt, &updatedAt); err != nil {
		return WorkCase{}, err
	}
	c.Status = CaseStatus(status)
	c.SubjectRefs = jsonToStrings(subjectRefs)
	if c.SubjectRefs == nil {
		c.SubjectRefs = []string{}
	}
	c.Participants = []CaseParticipant{}
	if participants != "" && participants != "[]" {
		_ = json.Unmarshal([]byte(participants), &c.Participants)
	}
	c.ExpectedCloseAt = valToTimePtr(expectedCloseAt)
	c.ClosedAt = valToTimePtr(closedAt)
	c.CreatedAt = strToTime(createdAt)
	c.UpdatedAt = strToTime(updatedAt)
	return c, nil
}

func participantsToJSON(v []CaseParticipant) string {
	if len(v) == 0 {
		return "[]"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// validateCaseContent checks the structural invariants shared by Create and
// Update: CaseRef-safe identity and well-formed subject DomainRefs. It is
// deliberately domain-neutral — no value vocabulary is enforced.
func validateCaseContent(workspaceID, caseID, primarySubject string, subjectRefs []string, participants []CaseParticipant) error {
	if _, err := domainref.NewCaseRef(workspaceID, caseID, 0); err != nil {
		return err
	}
	if primarySubject != "" {
		if _, err := domainref.ParseDomainRef(primarySubject); err != nil {
			return err
		}
	}
	for _, ref := range subjectRefs {
		if _, err := domainref.ParseDomainRef(ref); err != nil {
			return err
		}
	}
	for i, p := range participants {
		if p.ID == "" || p.Kind == "" {
			return fmt.Errorf("%w: participant[%d] requires id and kind", ErrInvalidProjectEvent, i)
		}
	}
	return nil
}

// Create inserts a new WorkCase owned by projectID and appends event in the
// same transaction. The case always starts open with version 1; ID is assigned
// when empty. Title is required.
func (s *WorkCaseStore) Create(projectID string, c WorkCase, event ProjectEvent) (WorkCase, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return WorkCase{}, err
	}
	defer tx.Rollback()
	created, err := s.CreateInTx(tx, projectID, c)
	if err != nil {
		return WorkCase{}, err
	}
	if _, err := appendProjectEventTx(tx, event, false); err != nil {
		return WorkCase{}, err
	}
	return created, tx.Commit()
}

// CreateInTx performs the Create write inside an open transaction so command
// handlers can commit the state change atomically with the gateway's
// idempotency and audit records (#323). Same invariants as Create.
func (s *WorkCaseStore) CreateInTx(tx *sql.Tx, projectID string, c WorkCase) (WorkCase, error) {
	if projectID == "" {
		return WorkCase{}, fmt.Errorf("%w: project_id is required", ErrInvalidProjectEvent)
	}
	if c.Title == "" {
		return WorkCase{}, fmt.Errorf("%w: title is required", ErrInvalidProjectEvent)
	}
	if c.ID == "" {
		c.ID = newID()
	}
	c.WorkspaceID = projectID
	c.Status = CaseStatusOpen
	c.Version = 1
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	if c.SubjectRefs == nil {
		c.SubjectRefs = []string{}
	}
	if c.Participants == nil {
		c.Participants = []CaseParticipant{}
	}
	if err := validateCaseContent(c.WorkspaceID, c.ID, c.PrimarySubject, c.SubjectRefs, c.Participants); err != nil {
		return WorkCase{}, err
	}
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM projects WHERE id = ?`, projectID).Scan(&exists); err != nil {
		return WorkCase{}, err
	}
	if exists == 0 {
		return WorkCase{}, ErrNotFound
	}
	if err := insertWorkCaseTx(tx, c); err != nil {
		return WorkCase{}, err
	}
	return c, nil
}

func insertWorkCaseTx(tx *sql.Tx, c WorkCase) error {
	_, err := tx.Exec(`
		INSERT INTO work_cases (id, project_id, case_definition, status, title, objective,
			owner, created_by, current_phase, primary_subject, subject_refs, participants,
			expected_close_at, version, close_reason, closed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.WorkspaceID, c.CaseDefinition, string(c.Status), c.Title, c.Objective,
		c.Owner, c.CreatedBy, c.CurrentPhase, c.PrimarySubject,
		stringsToJSON(c.SubjectRefs), participantsToJSON(c.Participants),
		timePtrToVal(c.ExpectedCloseAt), c.Version, c.CloseReason,
		timePtrToVal(c.ClosedAt), timeToStr(c.CreatedAt), timeToStr(c.UpdatedAt))
	return err
}

// Get returns one WorkCase by id. ok=false when unknown.
func (s *WorkCaseStore) Get(id string) (WorkCase, bool, error) {
	row, err := scanWorkCase(s.db.sql.QueryRow(
		`SELECT `+workCaseCols+` FROM work_cases WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return WorkCase{}, false, nil
	}
	if err != nil {
		return WorkCase{}, false, err
	}
	return row, true, nil
}

// List returns the workspace's cases, newest first. Pass status "" to skip the
// status filter.
func (s *WorkCaseStore) List(projectID string, status CaseStatus) ([]WorkCase, error) {
	query := `SELECT ` + workCaseCols + ` FROM work_cases WHERE project_id = ?`
	args := []any{projectID}
	if status != "" {
		if !status.Valid() {
			return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidProjectEvent, status)
		}
		query += ` AND status = ?`
		args = append(args, string(status))
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkCase{}
	for rows.Next() {
		c, err := scanWorkCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// loadCaseTx loads a case inside an open transaction and enforces the
// project scope (跨壳同一 ID、同一权限事实 §8: a case is only ever mutated through
// its owning workspace).
func loadCaseTx(tx *sql.Tx, projectID, id string) (WorkCase, error) {
	c, err := scanWorkCase(tx.QueryRow(`SELECT `+workCaseCols+` FROM work_cases WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return WorkCase{}, ErrNotFound
	}
	if err != nil {
		return WorkCase{}, err
	}
	if c.WorkspaceID != projectID {
		return WorkCase{}, ErrProjectMismatch
	}
	return c, nil
}

// checkCaseVersion returns ErrCaseVersionConflict unless expectedVersion
// matches the stored version (optimistic concurrency, §5.1 expectedVersion).
func checkCaseVersion(c WorkCase, expectedVersion int) error {
	if expectedVersion != c.Version {
		return fmt.Errorf("%w: expected version %d, current is %d",
			ErrCaseVersionConflict, expectedVersion, c.Version)
	}
	return nil
}

func updateWorkCaseTx(tx *sql.Tx, c WorkCase) error {
	_, err := tx.Exec(`
		UPDATE work_cases SET case_definition = ?, status = ?, title = ?, objective = ?,
			owner = ?, current_phase = ?, primary_subject = ?, subject_refs = ?,
			participants = ?, expected_close_at = ?, version = ?, close_reason = ?,
			closed_at = ?, updated_at = ?
		WHERE id = ?`,
		c.CaseDefinition, string(c.Status), c.Title, c.Objective,
		c.Owner, c.CurrentPhase, c.PrimarySubject, stringsToJSON(c.SubjectRefs),
		participantsToJSON(c.Participants), timePtrToVal(c.ExpectedCloseAt),
		c.Version, c.CloseReason, timePtrToVal(c.ClosedAt), timeToStr(c.UpdatedAt),
		c.ID)
	return err
}

// Update applies patch to a case under optimistic concurrency and appends
// event atomically. Status and CurrentPhase are never changed here —
// transitions go through Transition, phase advances only through a command
// (#323). The version is bumped by exactly one on success.
func (s *WorkCaseStore) Update(projectID, id string, patch WorkCasePatch, expectedVersion int, event ProjectEvent) (WorkCase, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return WorkCase{}, err
	}
	defer tx.Rollback()
	updated, err := s.UpdateInTx(tx, projectID, id, patch, expectedVersion)
	if err != nil {
		return WorkCase{}, err
	}
	if _, err := appendProjectEventTx(tx, event, false); err != nil {
		return WorkCase{}, err
	}
	return updated, tx.Commit()
}

// UpdateInTx performs the Update write inside an open transaction (command
// gateway seam, #323). Same invariants as Update.
func (s *WorkCaseStore) UpdateInTx(tx *sql.Tx, projectID, id string, patch WorkCasePatch, expectedVersion int) (WorkCase, error) {
	c, err := loadCaseTx(tx, projectID, id)
	if err != nil {
		return WorkCase{}, err
	}
	if err := checkCaseVersion(c, expectedVersion); err != nil {
		return WorkCase{}, err
	}
	if patch.Title != nil {
		if *patch.Title == "" {
			return WorkCase{}, fmt.Errorf("%w: title must not be empty", ErrInvalidProjectEvent)
		}
		c.Title = *patch.Title
	}
	if patch.Objective != nil {
		c.Objective = *patch.Objective
	}
	if patch.CaseDefinition != nil {
		c.CaseDefinition = *patch.CaseDefinition
	}
	if patch.Owner != nil {
		c.Owner = *patch.Owner
	}
	if patch.PrimarySubject != nil {
		c.PrimarySubject = *patch.PrimarySubject
	}
	if patch.SubjectRefs != nil {
		refs := *patch.SubjectRefs
		if refs == nil {
			refs = []string{}
		}
		c.SubjectRefs = refs
	}
	if patch.Participants != nil {
		ps := *patch.Participants
		if ps == nil {
			ps = []CaseParticipant{}
		}
		c.Participants = ps
	}
	if patch.ExpectedCloseAt != nil {
		t := *patch.ExpectedCloseAt
		c.ExpectedCloseAt = &t
	}
	if err := validateCaseContent(c.WorkspaceID, c.ID, c.PrimarySubject, c.SubjectRefs, c.Participants); err != nil {
		return WorkCase{}, err
	}
	c.Version++
	c.UpdatedAt = time.Now().UTC()
	if err := updateWorkCaseTx(tx, c); err != nil {
		return WorkCase{}, err
	}
	return c, nil
}

// SetPhaseInTx advances the application phase projection inside an open
// transaction under optimistic concurrency. This is the ONLY store path that
// writes current_phase (#323: 禁止直接修改 Case phase) — it is reachable
// exclusively through the registered workcase.set_phase command, so every
// phase advance carries an actor, an idempotency key and an audit trail.
// The kernel still never interprets the phase value.
func (s *WorkCaseStore) SetPhaseInTx(tx *sql.Tx, projectID, id, phase string, expectedVersion int) (WorkCase, error) {
	c, err := loadCaseTx(tx, projectID, id)
	if err != nil {
		return WorkCase{}, err
	}
	if err := checkCaseVersion(c, expectedVersion); err != nil {
		return WorkCase{}, err
	}
	c.CurrentPhase = phase
	c.Version++
	c.UpdatedAt = time.Now().UTC()
	if err := updateWorkCaseTx(tx, c); err != nil {
		return WorkCase{}, err
	}
	return c, nil
}

// Transition moves a case to the next lifecycle status under optimistic
// concurrency and appends event atomically. Rules:
//
//   - the move must be in the legal matrix (CanTransition); every move out of
//     a terminal state — and same-state no-ops — are rejected;
//   - expectedVersion must match the stored version;
//   - entering a terminal state stamps ClosedAt and stores reason as CloseReason.
func (s *WorkCaseStore) Transition(projectID, id string, to CaseStatus, reason string, expectedVersion int, event ProjectEvent) (WorkCase, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return WorkCase{}, err
	}
	defer tx.Rollback()
	updated, err := s.TransitionInTx(tx, projectID, id, to, reason, expectedVersion)
	if err != nil {
		return WorkCase{}, err
	}
	if _, err := appendProjectEventTx(tx, event, false); err != nil {
		return WorkCase{}, err
	}
	return updated, tx.Commit()
}

// TransitionInTx performs the lifecycle move inside an open transaction
// (command gateway seam, #323). Same rules as Transition.
func (s *WorkCaseStore) TransitionInTx(tx *sql.Tx, projectID, id string, to CaseStatus, reason string, expectedVersion int) (WorkCase, error) {
	if !to.Valid() {
		return WorkCase{}, fmt.Errorf("%w: unknown status %q", ErrInvalidCaseTransition, to)
	}
	c, err := loadCaseTx(tx, projectID, id)
	if err != nil {
		return WorkCase{}, err
	}
	if !CanTransition(c.Status, to) {
		return WorkCase{}, fmt.Errorf("%w: %s → %s is not allowed",
			ErrInvalidCaseTransition, c.Status, to)
	}
	if err := checkCaseVersion(c, expectedVersion); err != nil {
		return WorkCase{}, err
	}
	now := time.Now().UTC()
	c.Status = to
	if to.Terminal() {
		c.CloseReason = reason
		c.ClosedAt = &now
	}
	c.Version++
	c.UpdatedAt = now
	if err := updateWorkCaseTx(tx, c); err != nil {
		return WorkCase{}, err
	}
	return c, nil
}

// Delete removes a case and all of its links, appending event atomically.
func (s *WorkCaseStore) Delete(projectID, id string, event ProjectEvent) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.DeleteInTx(tx, projectID, id); err != nil {
		return err
	}
	if _, err := appendProjectEventTx(tx, event, false); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteInTx performs the deletion inside an open transaction (command
// gateway seam, #323). Same rules as Delete.
func (s *WorkCaseStore) DeleteInTx(tx *sql.Tx, projectID, id string) error {
	if _, err := loadCaseTx(tx, projectID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM work_case_links WHERE case_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM work_cases WHERE id = ?`, id); err != nil {
		return err
	}
	return nil
}

// ── associations (Task / Session / ArtifactRef, §4.3) ───────────────────────

// Link attaches targetID to the case as kind under optimistic concurrency and
// appends event atomically. Task and session targets must exist and belong to
// the case's workspace; artifact refs are opaque strings (the kernel stores
// the reference, the owning application interprets it). Duplicate links are
// rejected with ErrDuplicate.
func (s *WorkCaseStore) Link(projectID, caseID string, kind CaseLinkKind, targetID string, expectedVersion int, event ProjectEvent) (WorkCase, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return WorkCase{}, err
	}
	defer tx.Rollback()
	updated, err := s.LinkInTx(tx, projectID, caseID, kind, targetID, expectedVersion)
	if err != nil {
		return WorkCase{}, err
	}
	if _, err := appendProjectEventTx(tx, event, false); err != nil {
		return WorkCase{}, err
	}
	return updated, tx.Commit()
}

// LinkInTx performs the association write inside an open transaction (command
// gateway seam, #323). Same rules as Link.
func (s *WorkCaseStore) LinkInTx(tx *sql.Tx, projectID, caseID string, kind CaseLinkKind, targetID string, expectedVersion int) (WorkCase, error) {
	if !kind.Valid() {
		return WorkCase{}, fmt.Errorf("%w: unknown link kind %q", ErrInvalidProjectEvent, kind)
	}
	if targetID == "" {
		return WorkCase{}, fmt.Errorf("%w: link target_id is required", ErrInvalidProjectEvent)
	}
	c, err := loadCaseTx(tx, projectID, caseID)
	if err != nil {
		return WorkCase{}, err
	}
	if err := checkCaseVersion(c, expectedVersion); err != nil {
		return WorkCase{}, err
	}
	if err := validateLinkTargetTx(tx, projectID, kind, targetID); err != nil {
		return WorkCase{}, err
	}
	res, err := tx.Exec(`
		INSERT OR IGNORE INTO work_case_links (case_id, link_kind, target_id, created_at)
		VALUES (?, ?, ?, ?)`, caseID, string(kind), targetID, timeToStr(time.Now().UTC()))
	if err != nil {
		return WorkCase{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return WorkCase{}, ErrDuplicate
	}
	c.Version++
	c.UpdatedAt = time.Now().UTC()
	if err := updateWorkCaseTx(tx, c); err != nil {
		return WorkCase{}, err
	}
	return c, nil
}

// validateLinkTargetTx enforces workspace ownership for task/session targets.
func validateLinkTargetTx(tx *sql.Tx, projectID string, kind CaseLinkKind, targetID string) error {
	switch kind {
	case CaseLinkTask:
		var taskProject string
		err := tx.QueryRow(`SELECT project_id FROM project_items WHERE id = ?`, targetID).Scan(&taskProject)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if taskProject != projectID {
			return ErrProjectMismatch
		}
	case CaseLinkSession:
		var sessionProject string
		err := tx.QueryRow(`SELECT project_id FROM sessions WHERE id = ?`, targetID).Scan(&sessionProject)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// Legacy sessions may carry an empty project scope; only a *different*
		// non-empty owner is a cross-workspace violation (§7.1).
		if sessionProject != "" && sessionProject != projectID {
			return ErrProjectMismatch
		}
	}
	return nil
}

// Unlink detaches targetID from the case under optimistic concurrency and
// appends event atomically. Missing links are rejected with ErrNotFound.
func (s *WorkCaseStore) Unlink(projectID, caseID string, kind CaseLinkKind, targetID string, expectedVersion int, event ProjectEvent) (WorkCase, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return WorkCase{}, err
	}
	defer tx.Rollback()
	updated, err := s.UnlinkInTx(tx, projectID, caseID, kind, targetID, expectedVersion)
	if err != nil {
		return WorkCase{}, err
	}
	if _, err := appendProjectEventTx(tx, event, false); err != nil {
		return WorkCase{}, err
	}
	return updated, tx.Commit()
}

// UnlinkInTx performs the association removal inside an open transaction
// (command gateway seam, #323). Same rules as Unlink.
func (s *WorkCaseStore) UnlinkInTx(tx *sql.Tx, projectID, caseID string, kind CaseLinkKind, targetID string, expectedVersion int) (WorkCase, error) {
	if !kind.Valid() {
		return WorkCase{}, fmt.Errorf("%w: unknown link kind %q", ErrInvalidProjectEvent, kind)
	}
	c, err := loadCaseTx(tx, projectID, caseID)
	if err != nil {
		return WorkCase{}, err
	}
	if err := checkCaseVersion(c, expectedVersion); err != nil {
		return WorkCase{}, err
	}
	res, err := tx.Exec(
		`DELETE FROM work_case_links WHERE case_id = ? AND link_kind = ? AND target_id = ?`,
		caseID, string(kind), targetID)
	if err != nil {
		return WorkCase{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return WorkCase{}, ErrNotFound
	}
	c.Version++
	c.UpdatedAt = time.Now().UTC()
	if err := updateWorkCaseTx(tx, c); err != nil {
		return WorkCase{}, err
	}
	return c, nil
}

// ListLinks returns all association edges of a case ordered by kind and
// creation time.
func (s *WorkCaseStore) ListLinks(caseID string) ([]CaseLink, error) {
	rows, err := s.db.sql.Query(`
		SELECT case_id, link_kind, target_id, created_at
		FROM work_case_links WHERE case_id = ?
		ORDER BY link_kind, created_at, target_id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CaseLink{}
	for rows.Next() {
		var l CaseLink
		var kind, createdAt string
		if err := rows.Scan(&l.CaseID, &kind, &l.TargetID, &createdAt); err != nil {
			return nil, err
		}
		l.Kind = CaseLinkKind(kind)
		l.CreatedAt = strToTime(createdAt)
		out = append(out, l)
	}
	return out, rows.Err()
}

// ── by-Case queries (task #322: Task 和 TaskRun 可按 Case 查询) ────────────────

// ListTasksByCase returns the tasks linked to a case, oldest first. Rows are
// plain task scans (children not hydrated), matching ListTasksByBusinessRef.
func (s *WorkCaseStore) ListTasksByCase(caseID string) ([]Task, error) {
	rows, err := s.db.sql.Query(`
		SELECT `+taskCols+`
		FROM project_items
		WHERE id IN (
			SELECT target_id FROM work_case_links WHERE case_id = ? AND link_kind = 'task'
		)
		ORDER BY created_at, id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTaskRunsByCase returns every execution/verification run of the case's
// linked tasks, newest first — TaskRuns join through the task association, no
// second copy is kept (§4.4: 不复制 ProjectItem).
func (s *WorkCaseStore) ListTaskRunsByCase(caseID string) ([]TaskRun, error) {
	rows, err := s.db.sql.Query(`
		SELECT `+taskRunCols+`
		FROM task_runs
		WHERE task_id IN (
			SELECT target_id FROM work_case_links WHERE case_id = ? AND link_kind = 'task'
		)
		ORDER BY created_at DESC, id DESC`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskRun{}
	for rows.Next() {
		run, err := scanTaskRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
