package govern

import (
	"encoding/json"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func parseStd(t *testing.T, s string) meta.Recurrence {
	t.Helper()
	if s == "" {
		t.Fatal("expected non-empty normalized recurrence")
	}
	var r meta.Recurrence
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("normalized JSON unmarshal: %v (%s)", err, s)
	}
	return r
}

func TestNormalizeGraphWeeklyDaysNoEnd(t *testing.T) {
	// The real MS To Do sample from the fusion work: every Sat & Sun, no end.
	raw := `{"pattern":{"type":"weekly","interval":1,"daysOfWeek":["saturday","sunday"],"firstDayOfWeek":"monday"},"range":{"type":"noEnd","startDate":"2023-10-14"}}`
	r := parseStd(t, NormalizeGraphRecurrence(raw))
	if r.Freq != "weekly" {
		t.Fatalf("freq = %q, want weekly", r.Freq)
	}
	if len(r.DaysOfWeek) != 2 || r.DaysOfWeek[0] != 6 || r.DaysOfWeek[1] != 0 {
		t.Fatalf("daysOfWeek = %v, want [6 0]", r.DaysOfWeek)
	}
	if r.Until != "" || r.Count != 0 {
		t.Fatalf("noEnd should carry no until/count, got until=%q count=%d", r.Until, r.Count)
	}
}

func TestNormalizeGraphRelativeMonthlyNumbered(t *testing.T) {
	raw := `{"pattern":{"type":"relativeMonthly","interval":1,"daysOfWeek":["monday"],"index":"first"},"range":{"type":"numbered","numberOfOccurrences":5}}`
	r := parseStd(t, NormalizeGraphRecurrence(raw))
	if r.Freq != "monthly" || r.WeekIndex != 1 {
		t.Fatalf("freq/weekIndex = %q/%d, want monthly/1", r.Freq, r.WeekIndex)
	}
	if len(r.DaysOfWeek) != 1 || r.DaysOfWeek[0] != 1 {
		t.Fatalf("daysOfWeek = %v, want [1]", r.DaysOfWeek)
	}
	if r.Count != 5 {
		t.Fatalf("count = %d, want 5", r.Count)
	}
}

func TestNormalizeGraphAbsoluteMonthlyEndDate(t *testing.T) {
	raw := `{"pattern":{"type":"absoluteMonthly","interval":2,"dayOfMonth":15},"range":{"type":"endDate","endDate":"2026-12-31"}}`
	r := parseStd(t, NormalizeGraphRecurrence(raw))
	if r.Freq != "monthly" || r.Monthday != 15 || r.Interval != 2 {
		t.Fatalf("got %+v, want monthly/day15/interval2", r)
	}
	if r.Until != "2026-12-31" {
		t.Fatalf("until = %q, want 2026-12-31", r.Until)
	}
}

func TestNormalizeGraphEmpty(t *testing.T) {
	if NormalizeGraphRecurrence("") != "" || NormalizeGraphRecurrence("null") != "" {
		t.Fatal("empty/null must normalize to empty")
	}
	if NormalizeGraphRecurrence(`{"pattern":{}}`) != "" {
		t.Fatal("missing pattern type must normalize to empty")
	}
}

func TestNormalizeRRULEWeeklyCount(t *testing.T) {
	r := parseStd(t, NormalizeRRULE("RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=10"))
	if r.Freq != "weekly" {
		t.Fatalf("freq = %q, want weekly", r.Freq)
	}
	if len(r.DaysOfWeek) != 2 || r.DaysOfWeek[0] != 1 || r.DaysOfWeek[1] != 3 {
		t.Fatalf("daysOfWeek = %v, want [1 3]", r.DaysOfWeek)
	}
	if r.Count != 10 {
		t.Fatalf("count = %d, want 10", r.Count)
	}
}

func TestNormalizeRRULEMonthlyByday(t *testing.T) {
	// First Monday of the month.
	r := parseStd(t, NormalizeRRULE("FREQ=MONTHLY;BYDAY=1MO"))
	if r.Freq != "monthly" || r.WeekIndex != 1 {
		t.Fatalf("got %+v, want monthly/weekIndex1", r)
	}
	if len(r.DaysOfWeek) != 1 || r.DaysOfWeek[0] != 1 {
		t.Fatalf("daysOfWeek = %v, want [1]", r.DaysOfWeek)
	}
}

func TestNormalizeRRULEUntil(t *testing.T) {
	r := parseStd(t, NormalizeRRULE("FREQ=DAILY;UNTIL=20261231T000000Z"))
	if r.Freq != "daily" || r.Until != "2026-12-31" {
		t.Fatalf("got freq=%q until=%q, want daily/2026-12-31", r.Freq, r.Until)
	}
}

func TestNormalizeRRULEEmpty(t *testing.T) {
	if NormalizeRRULE("") != "" || NormalizeRRULE("BYDAY=MO") != "" {
		t.Fatal("empty/missing FREQ must normalize to empty")
	}
}
