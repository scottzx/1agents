package agent

import (
	"testing"
	"time"
)

// Wednesday 2026-01-07 12:00 local — a stable anchor for the weekday math below
// (kept in time.Local so nextOccurrence's .Local() conversion is a no-op).
func anchorWed() time.Time { return time.Date(2026, 1, 7, 12, 0, 0, 0, time.Local) }

func TestNextOccurrenceMultiWeekday(t *testing.T) {
	after := anchorWed()
	// Every Saturday & Sunday. From Wed, the soonest is Sat 2026-01-10.
	got := nextOccurrence(after, &Recurrence{Freq: "weekly", DaysOfWeek: []int{6, 0}}).Local()
	if !got.After(after) {
		t.Fatalf("next %v not after %v", got, after)
	}
	if wd := got.Weekday(); wd != time.Saturday && wd != time.Sunday {
		t.Fatalf("weekday = %v, want Sat or Sun", wd)
	}
	if got.Month() != time.January || got.Day() != 10 {
		t.Fatalf("got %v, want 2026-01-10 (first Sat after Wed)", got)
	}
}

func TestNextOccurrenceRelativeMonth(t *testing.T) {
	after := anchorWed()
	// First Monday of the month at 09:00. Jan's first Monday (5th) already
	// passed → roll to Feb's first Monday, 2026-02-02.
	got := nextOccurrence(after, &Recurrence{Freq: "monthly", WeekIndex: 1, DaysOfWeek: []int{1}, At: "09:00"}).Local()
	if got.Weekday() != time.Monday {
		t.Fatalf("weekday = %v, want Monday", got.Weekday())
	}
	if got.Month() != time.February || got.Day() != 2 {
		t.Fatalf("got %v, want 2026-02-02 (first Monday of Feb)", got)
	}
	if got.Hour() != 9 || got.Minute() != 0 {
		t.Fatalf("time = %02d:%02d, want 09:00", got.Hour(), got.Minute())
	}
}

func TestNextOccurrenceIntervalDaily(t *testing.T) {
	after := anchorWed()
	// Every 3 days at midnight → today's midnight has passed, so +3 days.
	got := nextOccurrence(after, &Recurrence{Freq: "daily", Interval: 3}).Local()
	if got.Month() != time.January || got.Day() != 10 {
		t.Fatalf("got %v, want 2026-01-10 (+3 days)", got)
	}
}

func TestNextOccurrenceYearly(t *testing.T) {
	after := anchorWed()
	got := nextOccurrence(after, &Recurrence{Freq: "yearly", Month: 3, Monthday: 15}).Local()
	if got.Month() != time.March || got.Day() != 15 || got.Year() != 2026 {
		t.Fatalf("got %v, want 2026-03-15", got)
	}
}

// TestRecurrenceCountTerminates verifies the occurrence budget: a Count=2 rule
// respawns once (clone carries Count=1), then stops.
func TestRecurrenceCountTerminates(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	done := now.Add(-time.Hour)
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "cnt", Title: "两次", Description: "x", AcceptanceCriteria: "done",
			Status: TaskStatusCompleted, CompletedAt: &done,
			Recurrence: &Recurrence{Freq: "daily", At: "09:00", Count: 2},
			CreatedAt:  now.Add(-2 * time.Hour), UpdatedAt: now}),
	})

	s.Tick()
	cfg, _ := store.Load(ref.Path)
	var clone *Task
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID != "cnt" && cfg.Tasks[i].ID != srcReqID {
			clone = &cfg.Tasks[i]
		}
	}
	if clone == nil || clone.Recurrence == nil || clone.Recurrence.Count != 1 {
		t.Fatalf("first respawn should carry Count=1, got %+v", clone)
	}

	// Complete the clone; the next tick sees Count<=1 and must NOT respawn.
	clone.Status = TaskStatusCompleted
	clone.CompletedAt = &done
	saveTasks(t, store, ref.Path, cfg.Tasks)
	s.Tick()
	cfg, _ = store.Load(ref.Path)
	if len(cfg.Tasks) != 3 {
		t.Fatalf("tasks = %d, want 3 (no respawn past the count budget)", len(cfg.Tasks))
	}
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == clone.ID && cfg.Tasks[i].Recurrence != nil {
			t.Fatalf("exhausted clone should lose its recurrence")
		}
	}
}

// TestRecurrenceUntilTerminates verifies the until bound: a completed rule whose
// next run falls past `until` does not respawn.
func TestRecurrenceUntilTerminates(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	done := now.Add(-time.Hour)
	past := now.AddDate(0, 0, -2).Format("2006-01-02")
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "exp", Title: "过期", Description: "x", AcceptanceCriteria: "done",
			Status: TaskStatusCompleted, CompletedAt: &done,
			Recurrence: &Recurrence{Freq: "daily", At: "09:00", Until: past},
			CreatedAt:  now.Add(-2 * time.Hour), UpdatedAt: now}),
	})
	s.Tick()
	cfg, _ := store.Load(ref.Path)
	if len(cfg.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (no respawn past until)", len(cfg.Tasks))
	}
}
