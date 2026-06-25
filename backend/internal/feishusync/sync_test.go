package feishusync

import (
	"context"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// fakeClient is an in-memory BitableClient for deterministic tests.
type fakeClient struct {
	records map[string]Record // record_id → record
	seq     int
	clock   time.Time
}

func newFakeClient(clock time.Time) *fakeClient {
	return &fakeClient{records: map[string]Record{}, clock: clock}
}

func (f *fakeClient) ListRecords(_ context.Context, _, _ string) ([]Record, error) {
	out := make([]Record, 0, len(f.records))
	for _, r := range f.records {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeClient) CreateRecord(_ context.Context, _, _ string, fields map[string]interface{}) (Record, error) {
	f.seq++
	id := "rec" + itoa(f.seq)
	rec := Record{RecordID: id, Fields: cloneFields(fields), LastModified: f.clock}
	f.records[id] = rec
	return rec, nil
}

func (f *fakeClient) UpdateRecord(_ context.Context, _, _, recordID string, fields map[string]interface{}) (Record, error) {
	rec := f.records[recordID]
	rec.RecordID = recordID
	rec.Fields = cloneFields(fields)
	rec.LastModified = f.clock
	f.records[recordID] = rec
	return rec, nil
}

func cloneFields(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func sampleTask(id, title string, updated time.Time) meta.Task {
	return meta.Task{
		ID:        id,
		Title:     title,
		Number:    1,
		Status:    meta.TaskStatusPending,
		Priority:  meta.PriorityMedium,
		Type:      meta.TaskTypeTask,
		Labels:    []string{"backend"},
		UpdatedAt: updated,
		CreatedAt: updated,
	}
}

func TestPushCreatesThenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	fc := newFakeClient(now)
	s := NewSyncer(fc, NewMemBindingStore(), "appA", "tbl1")

	tasks := []meta.Task{sampleTask("t1", "first", now)}

	r1, err := s.Push(ctx, tasks)
	if err != nil {
		t.Fatalf("push1: %v", err)
	}
	if r1.Created != 1 || r1.Updated != 0 {
		t.Fatalf("push1 = %+v, want 1 created", r1)
	}
	if len(fc.records) != 1 {
		t.Fatalf("expected 1 remote record, got %d", len(fc.records))
	}

	// Re-push with no local change → must skip, not duplicate.
	r2, err := s.Push(ctx, tasks)
	if err != nil {
		t.Fatalf("push2: %v", err)
	}
	if r2.Created != 0 || r2.Skipped != 1 {
		t.Fatalf("push2 = %+v, want skip", r2)
	}
	if len(fc.records) != 1 {
		t.Fatalf("re-push duplicated rows: %d", len(fc.records))
	}
}

func TestPushUpdatesOnLocalChange(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	fc := newFakeClient(now)
	s := NewSyncer(fc, NewMemBindingStore(), "appA", "tbl1")

	tasks := []meta.Task{sampleTask("t1", "first", now)}
	if _, err := s.Push(ctx, tasks); err != nil {
		t.Fatalf("push1: %v", err)
	}

	// Local edit advances UpdatedAt.
	tasks[0].Title = "first-edited"
	tasks[0].UpdatedAt = now.Add(time.Minute)
	fc.clock = now.Add(time.Minute)

	r, err := s.Push(ctx, tasks)
	if err != nil {
		t.Fatalf("push2: %v", err)
	}
	if r.Updated != 1 {
		t.Fatalf("push2 = %+v, want 1 updated", r)
	}
	var got Record
	for _, rec := range fc.records {
		got = rec
	}
	if asText(got.Fields[ColTitle]) != "first-edited" {
		t.Fatalf("remote title = %q, want first-edited", got.Fields[ColTitle])
	}
}

func TestPullAppliesRemoteOnlyChange(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	fc := newFakeClient(now)
	bs := NewMemBindingStore()
	s := NewSyncer(fc, bs, "appA", "tbl1")

	tasks := []meta.Task{sampleTask("t1", "first", now)}
	if _, err := s.Push(ctx, tasks); err != nil {
		t.Fatalf("push: %v", err)
	}

	// Remote edit (only remote moved): bump title + LastModified.
	var recID string
	for id := range fc.records {
		recID = id
	}
	rec := fc.records[recID]
	rec.Fields[ColTitle] = "remote-edited"
	rec.LastModified = now.Add(time.Minute)
	fc.records[recID] = rec

	changed, res, err := s.Pull(ctx, tasks)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if res.Updated != 1 || len(res.Conflicts) != 0 {
		t.Fatalf("pull res = %+v, want 1 updated, 0 conflicts", res)
	}
	if len(changed) != 1 || changed[0].Title != "remote-edited" {
		t.Fatalf("changed = %+v, want title remote-edited", changed)
	}
}

func TestPullLocalFirstOnConflict(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	fc := newFakeClient(now)
	bs := NewMemBindingStore()
	s := NewSyncer(fc, bs, "appA", "tbl1")

	tasks := []meta.Task{sampleTask("t1", "first", now)}
	if _, err := s.Push(ctx, tasks); err != nil {
		t.Fatalf("push: %v", err)
	}

	// Both sides change after the last sync.
	later := now.Add(time.Minute)
	tasks[0].Title = "local-edited"
	tasks[0].UpdatedAt = later

	var recID string
	for id := range fc.records {
		recID = id
	}
	rec := fc.records[recID]
	rec.Fields[ColTitle] = "remote-edited"
	rec.LastModified = later
	fc.records[recID] = rec

	changed, res, err := s.Pull(ctx, tasks)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(res.Conflicts) != 1 || res.Updated != 0 {
		t.Fatalf("pull res = %+v, want 1 conflict, 0 updated", res)
	}
	if len(changed) != 0 {
		t.Fatalf("local-first must not apply remote: changed = %+v", changed)
	}
	if res.Conflicts[0].TaskID != "t1" {
		t.Fatalf("conflict task = %q, want t1", res.Conflicts[0].TaskID)
	}
}

func TestPullCreatesRemoteOnlyTask(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	fc := newFakeClient(now)
	bs := NewMemBindingStore()
	s := NewSyncer(fc, bs, "appA", "tbl1")
	s.Now = func() time.Time { return now }

	// A row created directly in Feishu with no local counterpart.
	fc.records["recX"] = Record{
		RecordID:     "recX",
		Fields:       map[string]interface{}{ColTitle: "born-in-feishu", ColStatus: "pending"},
		LastModified: now,
	}

	changed, res, err := s.Pull(ctx, nil)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("pull res = %+v, want 1 created", res)
	}
	if len(changed) != 1 || changed[0].Title != "born-in-feishu" || changed[0].CreatedBy != "feishu-sync" {
		t.Fatalf("created task = %+v", changed)
	}
	if _, ok := bs.ByRecordID("recX"); !ok {
		t.Fatalf("binding not recorded for new task")
	}
}

func TestTaskToFieldsAndBackRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	start := now.Add(24 * time.Hour)
	src := meta.Task{
		ID: "t1", Title: "round", Number: 7,
		Status: meta.TaskStatusRunning, Priority: meta.PriorityHigh,
		Assignee: "claudecode", Labels: []string{"a", "b"},
		Milestone: "M1", Sprint: "S2", Type: meta.TaskTypeBug,
		Description: "body", PlannedStart: &start, UpdatedAt: now,
	}
	fields := TaskToFields(src)
	if fields[ColNumber] != 7 {
		t.Fatalf("number col = %v", fields[ColNumber])
	}
	if fields[ColPlannedStart] != start.UnixMilli() {
		t.Fatalf("planned start col = %v", fields[ColPlannedStart])
	}

	// Apply onto an empty task (writable fields only; number/updatedAt readonly).
	var dst meta.Task
	rec := Record{Fields: fields}
	if !ApplyRecordToTask(&dst, rec) {
		t.Fatal("expected changes applied")
	}
	if dst.Title != "round" || string(dst.Status) != "running" ||
		string(dst.Priority) != "high" || dst.Assignee != "claudecode" ||
		dst.Milestone != "M1" || dst.Sprint != "S2" || string(dst.Type) != "bug" ||
		dst.Description != "body" {
		t.Fatalf("round-trip mismatch: %+v", dst)
	}
	if !equalStrings(dst.Labels, []string{"a", "b"}) {
		t.Fatalf("labels = %v", dst.Labels)
	}
	// Read-only columns must NOT leak into the local task.
	if dst.Number != 0 {
		t.Fatalf("read-only number leaked: %d", dst.Number)
	}
}

func TestApplyIgnoresReadOnlyColumns(t *testing.T) {
	t1 := meta.Task{Number: 5}
	rec := Record{Fields: map[string]interface{}{
		ColNumber:    999,
		ColUpdatedAt: int64(123),
	}}
	if ApplyRecordToTask(&t1, rec) {
		t.Fatal("read-only-only record must report no change")
	}
	if t1.Number != 5 {
		t.Fatalf("number changed from read-only column: %d", t1.Number)
	}
}
