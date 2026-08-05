package agent

// Personal Shell cross-shell work aggregation (task #329, requirement #319;
// docs/architecture/enterprise-foundation-v1.0.0.md §8 / D7 个人工作台: "增强跨
// WorkCase 的多任务、多会话与待办聚合", data source "运行时内核 Query + 有权限的
// 领域摘要 Query").
//
// This is a strictly read-only Personal Shell read model. It aggregates the
// user's executable work across every WorkCase / domain into one paginated,
// filterable, sortable list:
//
//   - my tasks, running TaskRuns, items awaiting human input/approval,
//     failed, blocked, and due-soon items;
//   - each item joined to its WorkCase (CaseRef) and to the owning domain's
//     read-only summary (DomainRef) with a deep link back to the owning
//     shell / case / subject.
//
// Boundary rules enforced here (§3.4, §5, §7):
//
//   - Kernel facts only. Tasks, TaskRuns and WorkCases are read through the
//     kernel stores (internal/meta). This file opens NO domain table.
//   - Domain summaries resolve exclusively through the domainref Query
//     registry (§4.2): the owning domain's QueryProvider is the only reader of
//     its objects. When a provider is absent, denies access, or cannot find
//     the object, the item carries a restricted placeholder — never a leak.
//   - Canonical identity. Every item exposes the kernel Task's own id/status,
//     unchanged by which Product Shell renders it (§8: 三个壳读取同一 Task ID).

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/scottzx/1Agents/backend/internal/domainref"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// ShellPersonalID is the Personal Shell identifier (appregistry.ShellPersonal).
// Referenced by literal to avoid an agent → appregistry dependency.
const ShellPersonalID = "personal"

// Attention buckets an aggregated work item can belong to. A single task may
// carry several (e.g. running + due_soon). "open" is the residual lane for
// items that match no attention bucket.
const (
	aggBucketRunning  = "running"
	aggBucketAwaiting = "awaiting"
	aggBucketFailed   = "failed"
	aggBucketBlocked  = "blocked"
	aggBucketDueSoon  = "due_soon"
	aggBucketOpen     = "open"
	aggBucketAll      = "all"
)

// dueSoonWindow is how far ahead a due date counts as 即将逾期.
const dueSoonWindow = 7 * 24 * time.Hour

// defaultAggregateLimit / maxAggregateLimit bound pagination.
const (
	defaultAggregateLimit = 50
	maxAggregateLimit     = 200
)

// bucketSalience ranks buckets so the items that most need attention float to
// the top of the default ordering: awaiting > failed > blocked > running >
// due_soon > open.
func bucketSalience(bucket string) int {
	switch bucket {
	case aggBucketAwaiting:
		return 0
	case aggBucketFailed:
		return 1
	case aggBucketBlocked:
		return 2
	case aggBucketRunning:
		return 3
	case aggBucketDueSoon:
		return 4
	default:
		return 5
	}
}

// itemSalience is the smallest (most urgent) salience among the item's buckets.
func itemSalience(it *AggregateWorkItem) int {
	best := bucketSalience(aggBucketOpen)
	for _, b := range it.Buckets {
		if s := bucketSalience(b); s < best {
			best = s
		}
	}
	return best
}

// SubjectSummary is the read-only domain summary for an item's DomainRef, or a
// restricted placeholder when the owning domain cannot / will not reveal it.
type SubjectSummary struct {
	Ref       string `json:"ref"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	// Available is false when the summary is a restricted placeholder.
	Available bool `json:"available"`
	// Populated only when Available.
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
	Link   string `json:"link,omitempty"`
	// RestrictedReason explains an unavailable summary. One of the domainref
	// contract codes (unknown_provider / permission_denied / not_found /
	// version_unsupported / invalid_ref) or "error".
	RestrictedReason string `json:"restrictedReason,omitempty"`
}

// RunSummary is the latest execution/verification run that drives an item's
// classification (running/failed/needs-human). Omitted when irrelevant.
type RunSummary struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Attempt    int       `json:"attempt"`
	NeedsHuman bool      `json:"needsHuman,omitempty"`
	ErrorText  string    `json:"errorText,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
}

// DeepLink carries everything the frontend needs to navigate back to the
// correct shell / case / subject / task for an item.
type DeepLink struct {
	// Shell is the Product Shell the item itself opens in (personal here).
	Shell string `json:"shell"`
	// Task coordinates (canonical kernel task).
	TaskWorkspaceID string `json:"taskWorkspaceId,omitempty"`
	TaskID          string `json:"taskId,omitempty"`
	TaskNumber      int    `json:"taskNumber,omitempty"`
	// CaseRef of the bound WorkCase, when any.
	CaseRef string `json:"caseRef,omitempty"`
	// SubjectRef and the shell that owns the subject's domain, when any.
	SubjectRef   string `json:"subjectRef,omitempty"`
	SubjectShell string `json:"subjectShell,omitempty"`
}

// AggregateWorkItem is one aggregated, cross-shell work item. Its ID/Status are
// the kernel Task's own — identical in every Product Shell (§8).
type AggregateWorkItem struct {
	ID       string          `json:"id"`
	Number   int             `json:"number,omitempty"`
	Title    string          `json:"title"`
	Status   meta.TaskStatus `json:"status"`
	Priority meta.Priority   `json:"priority,omitempty"`
	Type     meta.ItemType   `json:"type,omitempty"`
	Executor meta.TaskExecutor `json:"executor,omitempty"`
	Assignee string          `json:"assignee,omitempty"`

	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName,omitempty"`

	// Buckets lists the attention categories this item belongs to.
	Buckets []string `json:"buckets"`

	DueAt     *time.Time `json:"dueAt,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
	CreatedAt time.Time  `json:"createdAt"`

	// WorkCase binding, when the task is linked to a case.
	CaseRef    string `json:"caseRef,omitempty"`
	CaseTitle  string `json:"caseTitle,omitempty"`
	CaseStatus string `json:"caseStatus,omitempty"`

	// Domain subject summary (DomainRef), resolved read-only.
	Subject *SubjectSummary `json:"subject,omitempty"`
	// Latest relevant run, when one drives the classification.
	Run *RunSummary `json:"run,omitempty"`

	DeepLink DeepLink `json:"deepLink"`
}

// PersonalAggregateResponse is the paginated read model for
// GET /api/agent/personal/aggregate.
type PersonalAggregateResponse struct {
	Shell       string              `json:"shell"`
	Actor       string              `json:"actor"`
	GeneratedAt time.Time           `json:"generatedAt"`
	Total       int                 `json:"total"`
	Limit       int                 `json:"limit"`
	Offset      int                 `json:"offset"`
	HasMore     bool                `json:"hasMore"`
	Counts      map[string]int      `json:"counts"`
	Items       []AggregateWorkItem `json:"items"`
}

// aggregateDeps bundles the kernel stores + the domain query registry the
// aggregation reads from. All are injected so the pure core is testable.
type aggregateDeps struct {
	tasksStore *meta.TaskStore
	caseStore  *meta.WorkCaseStore
	runStore   *meta.TaskRunStore
	registry   *domainref.Registry
}

// aggregateOptions carries the parsed request parameters + evaluation time.
type aggregateOptions struct {
	Actor     string
	Workspace string // filter by workspace id
	CaseID    string // filter by work case id
	Status    string // filter by task status
	Bucket    string // all | open | running | awaiting | failed | blocked | due_soon
	Sort      string // "" (salience) | updated | created | due | priority | title | status
	Dir       string // asc | desc
	Limit     int
	Offset    int
	Now       time.Time
	DueWindow time.Duration
}

// buildPersonalAggregate is the pure aggregation core. It reads kernel facts
// (tasks, runs, cases) and resolves domain summaries through the registry; it
// never opens a domain table. Split out from the HTTP handler for unit testing.
func buildPersonalAggregate(d aggregateDeps, opts aggregateOptions) (PersonalAggregateResponse, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.DueWindow <= 0 {
		opts.DueWindow = dueSoonWindow
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultAggregateLimit
	}
	if opts.Limit > maxAggregateLimit {
		opts.Limit = maxAggregateLimit
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.Bucket == "" {
		opts.Bucket = aggBucketAll
	}

	items, err := collectWorkItems(d, opts)
	if err != nil {
		return PersonalAggregateResponse{}, err
	}

	// Base set: workspace / case / status filters already applied during
	// collection (but NOT the bucket filter), so bucket counts reflect the full
	// distribution under the other filters.
	base := items
	counts := bucketCounts(base)

	// Apply the bucket filter, then sort, then paginate.
	filtered := filterByBucket(base, opts.Bucket)
	sortItems(filtered, opts.Sort, opts.Dir)

	total := len(filtered)
	end := opts.Offset + opts.Limit
	if end > total {
		end = total
	}
	page := []AggregateWorkItem{}
	if opts.Offset < total {
		page = filtered[opts.Offset:end]
	}

	return PersonalAggregateResponse{
		Shell:       ShellPersonalID,
		Actor:       opts.Actor,
		GeneratedAt: opts.Now,
		Total:       total,
		Limit:       opts.Limit,
		Offset:      opts.Offset,
		HasMore:     end < total,
		Counts:      counts,
		Items:       page,
	}, nil
}

// collectWorkItems walks every active/system workspace, classifies each
// executable task, joins it to its WorkCase and resolves the case's domain
// subject through the registry.
func collectWorkItems(d aggregateDeps, opts aggregateOptions) ([]AggregateWorkItem, error) {
	projects, err := d.tasksStore.ListProjects()
	if err != nil {
		return nil, err
	}

	items := []AggregateWorkItem{}
	for _, p := range projects {
		// Only live workspaces contribute, mirroring the agenda/dashboard scope.
		if p.Status != meta.ProjectStatusActive && p.Status != meta.ProjectStatusSystem {
			continue
		}
		if opts.Workspace != "" && p.ID != opts.Workspace {
			continue
		}

		// Case binding for this workspace: caseID → case, taskID → caseID.
		caseByID, taskCase, err := workspaceCaseIndex(d.caseStore, p.ID)
		if err != nil {
			return nil, err
		}

		cfg, err := d.tasksStore.Load(p.WorkspacePath)
		if err != nil {
			// A workspace whose meta failed to load contributes nothing rather
			// than failing the whole aggregate (partial availability).
			continue
		}
		for i := range cfg.Tasks {
			t := &cfg.Tasks[i]
			if !includeTask(t) {
				continue
			}
			if opts.Status != "" && string(t.Status) != opts.Status {
				continue
			}
			// Case filter: keep only items bound to the requested case.
			if opts.CaseID != "" && taskCase[t.ID] != opts.CaseID {
				continue
			}
			item := newWorkItem(t, &p, taskCase, caseByID, d, opts)
			items = append(items, item)
		}
	}
	return items, nil
}

// workspaceCaseIndex builds caseID → WorkCase and taskID → caseID for a project.
func workspaceCaseIndex(caseStore *meta.WorkCaseStore, projectID string) (map[string]meta.WorkCase, map[string]string, error) {
	caseByID := map[string]meta.WorkCase{}
	taskCase := map[string]string{}
	if caseStore == nil {
		return caseByID, taskCase, nil
	}
	cases, err := caseStore.List(projectID, "")
	if err != nil {
		return nil, nil, err
	}
	for _, c := range cases {
		caseByID[c.ID] = c
		links, err := caseStore.ListLinks(c.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, l := range links {
			if l.Kind == meta.CaseLinkTask {
				taskCase[l.TargetID] = c.ID
			}
		}
	}
	return caseByID, taskCase, nil
}

// includeTask reports whether a task belongs in the personal work aggregate:
// executable, open, and not an unadopted AI suggestion.
func includeTask(t *meta.Task) bool {
	if t.Source == meta.TaskSourceAgent {
		return false // suggestions stay out until adopted
	}
	if t.IssueState == meta.IssueClosed {
		return false
	}
	if t.Type != "" && t.Type != meta.ItemTypeTask {
		return false // requirements/bugs/discussions are issue items, not work
	}
	switch t.Status {
	case meta.TaskStatusCompleted, meta.TaskStatusCancelled:
		return false
	}
	return true
}

// taskDueAt returns the task's due horizon: plannedEnd first, else scheduledAt.
func taskDueAt(t *meta.Task) *time.Time {
	if t.PlannedEnd != nil {
		return t.PlannedEnd
	}
	return t.ScheduledAt
}

// needsRun reports whether classification benefits from the latest run.
func needsRun(t *meta.Task) bool {
	switch t.Status {
	case meta.TaskStatusRunning, meta.TaskStatusFailed,
		meta.TaskStatusPendingReview, meta.TaskStatusAwaitingHuman,
		meta.TaskStatusBlocked, meta.TaskStatusNotReady, meta.TaskStatusQueued:
		return true
	}
	return false
}

// newWorkItem builds one AggregateWorkItem for a task.
func newWorkItem(t *meta.Task, p *meta.Project, taskCase map[string]string, caseByID map[string]meta.WorkCase, d aggregateDeps, opts aggregateOptions) AggregateWorkItem {
	item := AggregateWorkItem{
		ID:            t.ID,
		Number:        t.Number,
		Title:         t.Title,
		Status:        t.Status,
		Priority:      t.Priority,
		Type:          t.Type,
		Executor:      t.Executor,
		Assignee:      t.Assignee,
		WorkspaceID:   p.ID,
		WorkspaceName: p.Name,
		DueAt:         taskDueAt(t),
		UpdatedAt:     t.UpdatedAt,
		CreatedAt:     t.CreatedAt,
		Buckets:       []string{},
	}

	// Latest relevant run (drives running / failed / needs-human detection).
	var latestRun *meta.TaskRun
	if needsRun(t) && d.runStore != nil {
		if runs, err := d.runStore.ListByTask(t.ID); err == nil && len(runs) > 0 {
			r := runs[0]
			latestRun = &r
			item.Run = &RunSummary{
				ID:         r.ID,
				Kind:       string(r.Kind),
				Status:     string(r.Status),
				Attempt:    r.Attempt,
				NeedsHuman: r.Verdict != nil && r.Verdict.NeedsHuman,
				ErrorText:  r.ErrorText,
				StartedAt:  r.StartedAt,
			}
		}
	}

	// Classify into attention buckets.
	item.Buckets = classifyBuckets(t, latestRun, opts.Now, opts.DueWindow)

	// WorkCase binding + domain subject resolution.
	if caseID, ok := taskCase[t.ID]; ok {
		if c, ok := caseByID[caseID]; ok {
			ref, err := c.Ref()
			if err == nil {
				item.CaseRef = ref.String()
			} else {
				item.CaseRef = "case:" + c.WorkspaceID + ":" + c.ID
			}
			item.CaseTitle = c.Title
			item.CaseStatus = string(c.Status)
			item.Subject = resolveSubject(d.registry, subjectRefOf(c), opts)
		}
	}

	item.DeepLink = buildDeepLink(&item, t)
	return item
}

// subjectRefOf picks the case's primary subject DomainRef string, falling back
// to the first related subject ref. "" when the case has no subject.
func subjectRefOf(c meta.WorkCase) string {
	if c.PrimarySubject != "" {
		return c.PrimarySubject
	}
	if len(c.SubjectRefs) > 0 {
		return c.SubjectRefs[0]
	}
	return ""
}

// resolveSubject resolves a subject DomainRef through the domain Query registry
// into a summary, or a restricted placeholder when unavailable. It NEVER reads a
// domain table — the owning provider is the only reader (§4.2).
func resolveSubject(reg *domainref.Registry, refStr string, opts aggregateOptions) *SubjectSummary {
	if refStr == "" {
		return nil
	}
	ref, err := domainref.ParseDomainRef(refStr)
	if err != nil {
		return &SubjectSummary{
			Ref:              refStr,
			Available:        false,
			RestrictedReason: string(domainref.CodeInvalidRef),
		}
	}
	base := SubjectSummary{
		Ref:       ref.String(),
		Namespace: ref.Namespace,
		Type:      ref.Type,
		ID:        ref.ID,
	}
	if reg == nil {
		base.RestrictedReason = string(domainref.CodeUnknownProvider)
		return &base
	}
	summary, err := reg.Resolve(context.Background(), domainref.QueryRequest{
		Ref:   ref,
		Actor: opts.Actor,
	})
	if err != nil {
		code, ok := domainref.CodeOf(err)
		if !ok {
			code = "error"
		}
		base.RestrictedReason = string(code)
		return &base
	}
	base.Available = true
	base.Title = summary.Title
	base.Status = summary.Status
	base.Link = summary.Link
	return &base
}

// classifyBuckets returns the attention buckets a task belongs to.
func classifyBuckets(t *meta.Task, latestRun *meta.TaskRun, now time.Time, dueWindow time.Duration) []string {
	var buckets []string
	st := t.Status

	// Awaiting human input / approval.
	if st == meta.TaskStatusAwaitingHuman || st == meta.TaskStatusPendingReview ||
		(t.Executor == meta.TaskExecutorHuman && (st == meta.TaskStatusPending || st == meta.TaskStatusQueued)) ||
		(latestRun != nil && latestRun.Verdict != nil && latestRun.Verdict.NeedsHuman) {
		buckets = append(buckets, aggBucketAwaiting)
	}
	// Running.
	if st == meta.TaskStatusRunning || (latestRun != nil && latestRun.Status == meta.TaskRunRunning) {
		buckets = append(buckets, aggBucketRunning)
	}
	// Failed.
	if st == meta.TaskStatusFailed || (latestRun != nil && latestRun.Status == meta.TaskRunFailed) {
		buckets = append(buckets, aggBucketFailed)
	}
	// Blocked.
	if st == meta.TaskStatusBlocked || st == meta.TaskStatusNotReady {
		buckets = append(buckets, aggBucketBlocked)
	}
	// Due soon (includes already-overdue open work).
	if due := taskDueAt(t); due != nil && due.Before(now.Add(dueWindow)) {
		buckets = append(buckets, aggBucketDueSoon)
	}
	return buckets
}

// shellForNamespace maps a domain namespace to the Product Shell that owns it,
// for deep linking. Unknown namespaces return "" (no owning shell known).
func shellForNamespace(namespace string) string {
	switch namespace {
	case "presales":
		return "presales"
	case "commerce":
		return "commerce"
	default:
		return ""
	}
}

// buildDeepLink assembles the navigation coordinates for an item.
func buildDeepLink(item *AggregateWorkItem, t *meta.Task) DeepLink {
	dl := DeepLink{
		Shell:           ShellPersonalID,
		TaskWorkspaceID: item.WorkspaceID,
		TaskID:          item.ID,
		TaskNumber:      item.Number,
	}
	if item.CaseRef != "" {
		dl.CaseRef = item.CaseRef
	}
	if item.Subject != nil && item.Subject.Ref != "" {
		dl.SubjectRef = item.Subject.Ref
		dl.SubjectShell = shellForNamespace(item.Subject.Namespace)
	}
	return dl
}

// filterByBucket keeps items matching the requested bucket.
func filterByBucket(items []AggregateWorkItem, bucket string) []AggregateWorkItem {
	switch bucket {
	case "", aggBucketAll:
		return items
	case aggBucketOpen:
		out := []AggregateWorkItem{}
		for _, it := range items {
			if len(it.Buckets) == 0 {
				out = append(out, it)
			}
		}
		return out
	default:
		out := []AggregateWorkItem{}
		for _, it := range items {
			if hasBucket(it.Buckets, bucket) {
				out = append(out, it)
			}
		}
		return out
	}
}

func hasBucket(buckets []string, b string) bool {
	for _, x := range buckets {
		if x == b {
			return true
		}
	}
	return false
}

// bucketCounts tallies per-bucket counts over the base set. "all" is the base
// size; "open" counts items with no attention bucket.
func bucketCounts(items []AggregateWorkItem) map[string]int {
	counts := map[string]int{aggBucketAll: len(items), aggBucketOpen: 0}
	for _, it := range items {
		if len(it.Buckets) == 0 {
			counts[aggBucketOpen]++
		}
		for _, b := range it.Buckets {
			counts[b]++
		}
	}
	return counts
}

// sortItems orders items by the requested field; empty field = salience. When
// dir is empty a per-field natural direction is used: recency fields default to
// newest-first, every other field (including salience) defaults to ascending so
// the most urgent / soonest / highest-priority items surface first.
func sortItems(items []AggregateWorkItem, sortField, dir string) {
	less := func(a, b *AggregateWorkItem) bool {
		switch sortField {
		case "title":
			return a.Title < b.Title
		case "priority":
			ra, rb := meta.PriorityRank(a.Priority), meta.PriorityRank(b.Priority)
			if ra != rb {
				return ra < rb
			}
			return a.UpdatedAt.After(b.UpdatedAt)
		case "due":
			return dueBefore(a.DueAt, b.DueAt)
		case "created":
			return a.CreatedAt.Before(b.CreatedAt)
		case "status":
			return string(a.Status) < string(b.Status)
		case "updated":
			return a.UpdatedAt.Before(b.UpdatedAt)
		default: // salience, then due date, then recency
			sa, sb := itemSalience(a), itemSalience(b)
			if sa != sb {
				return sa < sb
			}
			if !dueEqual(a.DueAt, b.DueAt) {
				return dueBefore(a.DueAt, b.DueAt)
			}
			return a.UpdatedAt.After(b.UpdatedAt)
		}
	}
	effDir := dir
	if effDir == "" {
		switch sortField {
		case "updated", "created":
			effDir = "desc"
		default:
			effDir = "asc"
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if effDir == "asc" {
			return less(&items[i], &items[j])
		}
		return less(&items[j], &items[i])
	})
}

// dueBefore orders by due date with nil dues last.
func dueBefore(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.Before(*b)
}

func dueEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// HandlePersonalAggregate serves GET /api/agent/personal/aggregate — the
// Personal Shell cross-shell work aggregate (read-only).
func (h *Handler) HandlePersonalAggregate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.tasksStore == nil {
		http.Error(w, "task store is unavailable", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()

	limit := defaultAggregateLimit
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	actor := q.Get("actor")
	if actor == "" {
		actor = "user"
	}

	opts := aggregateOptions{
		Actor:     actor,
		Workspace: q.Get("workspace"),
		CaseID:    q.Get("case"),
		Status:    q.Get("status"),
		Bucket:    q.Get("bucket"),
		Sort:      q.Get("sort"),
		Dir:       q.Get("dir"),
		Limit:     limit,
		Offset:    offset,
		Now:       time.Now().UTC(),
		DueWindow: dueSoonWindow,
	}
	deps := aggregateDeps{
		tasksStore: h.tasksStore,
		caseStore:  h.workCaseStore,
		runStore:   h.taskRunStore,
		registry:   domainref.DefaultRegistry(),
	}
	resp, err := buildPersonalAggregate(deps, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}
