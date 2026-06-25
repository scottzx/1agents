package meta

import (
	"reflect"
	"testing"
	"time"
)

func TestParseRefs(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []Ref
	}{
		{"empty", "", nil},
		{"bare", "fixes #3 and relates to #10", []Ref{{Number: 3}, {Number: 10}}},
		{"dedupe + order", "#5 then #2 then #5", []Ref{{Number: 5}, {Number: 2}}},
		{"non-digit ignored", "see #bug or #todo but also #7", []Ref{{Number: 7}}},
		{"word-boundary guard", "v1.2#3 abc#4 but #8 counts", []Ref{{Number: 8}}},
		{"cross project", "needs `Web#42` first", []Ref{{Project: "Web", Number: 42}}},
		{
			"cross + same, no double count",
			"`My Proj#9` blocks #3",
			[]Ref{{Project: "My Proj", Number: 9}, {Number: 3}},
		},
		{"leading hash", "#1 at start", []Ref{{Number: 1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseRefs(c.text)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ParseRefs(%q) = %+v, want %+v", c.text, got, c.want)
			}
		})
	}
}

// seedGraphProject creates a project with three numbered tasks (#1..#3) and
// returns the store + their ids by number.
func seedGraphProject(t *testing.T) (*TaskStore, string, map[int]string) {
	t.Helper()
	db := newTestDB(t)
	s := NewTaskStore(db)
	ws := t.TempDir()
	now := time.Now().UTC()
	if err := db.EnsureProject("proj-1", "Alpha", ws); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := s.Save(ws, &TasksConfig{Tasks: []Task{
		{ID: "t1", Title: "root requirement", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "t2", Title: "work", Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now},
		{ID: "t3", Title: "more", Status: TaskStatusPending, CreatedAt: now.Add(2 * time.Second), UpdatedAt: now},
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return s, ws, map[int]string{1: "t1", 2: "t2", 3: "t3"}
}

func TestSyncRefLinksAddsRelates(t *testing.T) {
	s, ws, ids := seedGraphProject(t)

	// t2's description mentions #1 → a relates link to t1 is created.
	if err := s.UpdateDescription(ids[2], "implements part of #1"); err != nil {
		t.Fatalf("UpdateDescription: %v", err)
	}
	added, err := s.SyncRefLinks(ids[2])
	if err != nil {
		t.Fatalf("SyncRefLinks: %v", err)
	}
	if len(added) != 1 || added[0].Target != ids[1] || added[0].Rel != LinkRelates {
		t.Fatalf("added = %+v, want one relates→t1", added)
	}

	// Idempotent: a second sync over the same text adds nothing.
	again, err := s.SyncRefLinks(ids[2])
	if err != nil {
		t.Fatalf("SyncRefLinks again: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second sync added %+v, want none", again)
	}

	// Existing links are preserved when a new mention is added.
	if err := s.UpdateDescription(ids[2], "implements #1 and also touches #3"); err != nil {
		t.Fatalf("UpdateDescription 2: %v", err)
	}
	added, err = s.SyncRefLinks(ids[2])
	if err != nil {
		t.Fatalf("SyncRefLinks 3: %v", err)
	}
	if len(added) != 1 || added[0].Target != ids[3] {
		t.Fatalf("added = %+v, want one relates→t3", added)
	}
	got, _, _ := s.GetTask(ids[2])
	if len(got.Links) != 2 {
		t.Fatalf("t2 links = %+v, want 2 (t1 + t3)", got.Links)
	}

	// A self-reference and a dangling number are dropped.
	if err := s.UpdateDescription(ids[1], "this is #1 itself and a ghost #99"); err != nil {
		t.Fatalf("UpdateDescription self: %v", err)
	}
	added, err = s.SyncRefLinks(ids[1])
	if err != nil {
		t.Fatalf("SyncRefLinks self: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("self/dangling added %+v, want none", added)
	}
	_ = ws
}

func TestLinkGraphForBidirectional(t *testing.T) {
	s, _, ids := seedGraphProject(t)

	// t2 → t1 (relates), t3 → t1 (relates): t1 has two backlinks, no outgoing.
	for _, n := range []int{2, 3} {
		if err := s.UpdateDescription(ids[n], "relates to #1"); err != nil {
			t.Fatalf("UpdateDescription t%d: %v", n, err)
		}
		if _, err := s.SyncRefLinks(ids[n]); err != nil {
			t.Fatalf("SyncRefLinks t%d: %v", n, err)
		}
	}

	g, ok, err := s.LinkGraphFor(ids[1])
	if err != nil || !ok {
		t.Fatalf("LinkGraphFor(t1): ok=%v err=%v", ok, err)
	}
	if len(g.Outgoing) != 0 {
		t.Fatalf("t1 outgoing = %+v, want none", g.Outgoing)
	}
	if len(g.Incoming) != 2 {
		t.Fatalf("t1 incoming = %d edges, want 2 (t2, t3)", len(g.Incoming))
	}
	seen := map[string]bool{}
	for _, e := range g.Incoming {
		if e.Rel != LinkRelates {
			t.Fatalf("incoming rel = %q, want relates", e.Rel)
		}
		seen[e.Task.ID] = true
	}
	if !seen[ids[2]] || !seen[ids[3]] {
		t.Fatalf("incoming peers = %v, want t2 & t3", seen)
	}

	// The mirror view: t2 has one outgoing (→t1) and no backlinks.
	g2, ok, err := s.LinkGraphFor(ids[2])
	if err != nil || !ok {
		t.Fatalf("LinkGraphFor(t2): ok=%v err=%v", ok, err)
	}
	if len(g2.Outgoing) != 1 || g2.Outgoing[0].Task.ID != ids[1] {
		t.Fatalf("t2 outgoing = %+v, want one →t1", g2.Outgoing)
	}
	if len(g2.Incoming) != 0 {
		t.Fatalf("t2 incoming = %+v, want none", g2.Incoming)
	}

	// Unknown task → graceful not-found.
	if _, ok, err := s.LinkGraphFor("nope"); ok || err != nil {
		t.Fatalf("unknown task: ok=%v err=%v, want false/nil", ok, err)
	}
}
