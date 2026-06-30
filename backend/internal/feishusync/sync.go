package feishusync

import (
	"context"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Binding records the link between a local task and its Bitable row. It is the
// idempotency anchor: matching is by RecordID, never by title. SyncedAt mirrors
// the local UpdatedAt at the moment of the last successful push, so a later pull
// can tell "remote changed since we last pushed" from "no change".
type Binding struct {
	TaskID    string
	RecordID  string
	AppToken  string
	TableID   string
	SyncedAt  time.Time
}

// BindingStore persists task↔record bindings. It is an interface so the sync
// engine stays testable with an in-memory implementation; a DB-backed
// implementation (the issue's external_sync table) can be dropped in without
// touching the engine. Methods are keyed by provider-agnostic task id within a
// single (appToken, tableID) binding scope.
type BindingStore interface {
	// Get returns the binding for taskID, or ok=false if unbound.
	Get(taskID string) (Binding, bool)
	// ByRecordID returns the binding owning recordID, or ok=false.
	ByRecordID(recordID string) (Binding, bool)
	// Put upserts a binding.
	Put(b Binding) error
	// All returns every binding (used to detect locally-deleted tasks).
	All() []Binding
}

// MemBindingStore is an in-memory BindingStore for tests and dry runs.
type MemBindingStore struct {
	byTask   map[string]Binding
	byRecord map[string]Binding
}

func NewMemBindingStore() *MemBindingStore {
	return &MemBindingStore{
		byTask:   map[string]Binding{},
		byRecord: map[string]Binding{},
	}
}

func (m *MemBindingStore) Get(taskID string) (Binding, bool) { b, ok := m.byTask[taskID]; return b, ok }
func (m *MemBindingStore) ByRecordID(recordID string) (Binding, bool) {
	b, ok := m.byRecord[recordID]
	return b, ok
}
func (m *MemBindingStore) Put(b Binding) error {
	m.byTask[b.TaskID] = b
	if b.RecordID != "" {
		m.byRecord[b.RecordID] = b
	}
	return nil
}
func (m *MemBindingStore) All() []Binding {
	out := make([]Binding, 0, len(m.byTask))
	for _, b := range m.byTask {
		out = append(out, b)
	}
	return out
}

var _ BindingStore = (*MemBindingStore)(nil)

// Syncer drives push/pull/bidirectional sync for one project↔table binding.
type Syncer struct {
	Client   BitableClient
	Bindings BindingStore
	AppToken string
	TableID  string
	// Now is injectable for deterministic tests; defaults to time.Now.
	Now func() time.Time
}

// NewSyncer constructs a Syncer for a single (appToken, tableID) binding.
func NewSyncer(c BitableClient, b BindingStore, appToken, tableID string) *Syncer {
	return &Syncer{
		Client:   c,
		Bindings: b,
		AppToken: appToken,
		TableID:  tableID,
		Now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// PushResult summarizes a push pass.
type PushResult struct {
	Created int
	Updated int
	Skipped int // bound and unchanged since last sync
}

// Push sends local tasks to the Bitable. New tasks (no binding) are created;
// bound tasks whose UpdatedAt advanced past the last SyncedAt are updated; the
// rest are skipped. Idempotent: re-running with no local changes creates
// nothing. Returns the (possibly mutated) result and the set of task ids whose
// binding was just established/refreshed.
func (s *Syncer) Push(ctx context.Context, tasks []meta.Task) (PushResult, error) {
	var res PushResult
	for _, t := range tasks {
		fields := TaskToFields(t)
		b, bound := s.Bindings.Get(t.ID)
		if !bound || b.RecordID == "" {
			rec, err := s.Client.CreateRecord(ctx, s.AppToken, s.TableID, fields)
			if err != nil {
				return res, err
			}
			if err := s.Bindings.Put(Binding{
				TaskID: t.ID, RecordID: rec.RecordID,
				AppToken: s.AppToken, TableID: s.TableID, SyncedAt: t.UpdatedAt,
			}); err != nil {
				return res, err
			}
			res.Created++
			continue
		}
		// Bound: incremental — only push if the local task advanced.
		if !t.UpdatedAt.After(b.SyncedAt) {
			res.Skipped++
			continue
		}
		if _, err := s.Client.UpdateRecord(ctx, s.AppToken, s.TableID, b.RecordID, fields); err != nil {
			return res, err
		}
		b.SyncedAt = t.UpdatedAt
		if err := s.Bindings.Put(b); err != nil {
			return res, err
		}
		res.Updated++
	}
	return res, nil
}

// Conflict describes a row changed on BOTH sides since the last sync. Under the
// local-first policy the local version is kept (pushed back) and the conflict is
// reported for logging / optional user notification.
type Conflict struct {
	TaskID   string
	RecordID string
}

// PullResult summarizes a pull pass.
type PullResult struct {
	Updated   int        // existing local tasks changed from remote
	Created   int        // new local tasks created from remote-only rows
	Conflicts []Conflict // two-sided conflicts, resolved local-first
}

// Pull reads the Bitable and reconciles remote changes back into local tasks
// under the local-first policy:
//
//   - remote changed, local unchanged → apply remote to local (Updated)
//   - both changed                    → keep local, record a Conflict
//   - remote-only row (no binding)    → create a new local task (Created)
//
// `tasks` is the current local set, keyed by id. It returns the new/updated
// tasks to persist (only the ones that changed) plus the result summary. The
// caller is responsible for writing the returned tasks and bindings back to the
// store (kept out of the engine to keep it store-agnostic and unit-testable).
func (s *Syncer) Pull(ctx context.Context, tasks []meta.Task) ([]meta.Task, PullResult, error) {
	var res PullResult
	local := make(map[string]meta.Task, len(tasks))
	for _, t := range tasks {
		local[t.ID] = t
	}
	records, err := s.Client.ListRecords(ctx, s.AppToken, s.TableID)
	if err != nil {
		return nil, res, err
	}

	var out []meta.Task
	for _, rec := range records {
		b, bound := s.Bindings.ByRecordID(rec.RecordID)
		if !bound {
			// Remote-only row → create a new local task tagged as feishu-sourced.
			nt := s.recordToNewTask(rec)
			if err := s.Bindings.Put(Binding{
				TaskID: nt.ID, RecordID: rec.RecordID,
				AppToken: s.AppToken, TableID: s.TableID, SyncedAt: nt.UpdatedAt,
			}); err != nil {
				return nil, res, err
			}
			out = append(out, nt)
			res.Created++
			continue
		}
		t, ok := local[b.TaskID]
		if !ok {
			// Bound task vanished locally; skip (local-delete archival is a
			// separate concern, see issue Phase 2 notes).
			continue
		}
		localChanged := t.UpdatedAt.After(b.SyncedAt)
		remoteChanged := rec.LastModified.After(b.SyncedAt)
		if !remoteChanged {
			continue // nothing new remotely
		}
		if localChanged {
			// Both sides moved → local wins, report conflict, do not apply.
			res.Conflicts = append(res.Conflicts, Conflict{TaskID: t.ID, RecordID: rec.RecordID})
			continue
		}
		// Remote-only change → apply writable fields.
		if ApplyRecordToTask(&t, rec) {
			b.SyncedAt = t.UpdatedAt
			if err := s.Bindings.Put(b); err != nil {
				return nil, res, err
			}
			out = append(out, t)
			res.Updated++
		}
	}
	return out, res, nil
}

// recordToNewTask builds a fresh local task from a remote-only Bitable row.
func (s *Syncer) recordToNewTask(rec Record) meta.Task {
	now := s.now()
	t := meta.Task{
		ID:         "feishu-" + rec.RecordID,
		IssueState: meta.IssueOpen,
		Status:     meta.TaskStatusPending,
		CreatedBy:  "feishu-sync",
		CreatedAt:  now,
		UpdatedAt:  now,
		DependsOn:  []string{},
	}
	ApplyRecordToTask(&t, rec)
	if t.Title == "" {
		t.Title = "(untitled feishu record)"
	}
	t.UpdatedAt = now
	return t
}
