package meta

import (
	"reflect"
	"testing"
)

func TestIsReservedLabel(t *testing.T) {
	cases := map[string]bool{
		"p0":             true,
		"P0":             true, // case-insensitive
		"  needs-verify": true, // trimmed
		"auto-mergeable": true,
		"blocked":        true,
		"p3":             true,
		"backend":        false,
		"frontend":       false,
		"":               false,
		"needsverify":    false, // not the reserved spelling
	}
	for label, want := range cases {
		if got := IsReservedLabel(label); got != want {
			t.Errorf("IsReservedLabel(%q) = %v, want %v", label, got, want)
		}
	}
}

func TestSplitLabels(t *testing.T) {
	reserved, plain := SplitLabels([]string{"backend", "p0", "ui", "needs-verify", "P0"})
	wantReserved := []string{"p0", "needs-verify"} // "P0" deduped against "p0"
	wantPlain := []string{"backend", "ui"}
	if !reflect.DeepEqual(reserved, wantReserved) {
		t.Errorf("reserved = %v, want %v", reserved, wantReserved)
	}
	if !reflect.DeepEqual(plain, wantPlain) {
		t.Errorf("plain = %v, want %v", plain, wantPlain)
	}
}

func TestSplitLabelsEmpty(t *testing.T) {
	reserved, plain := SplitLabels(nil)
	if reserved != nil || plain != nil {
		t.Errorf("SplitLabels(nil) = (%v, %v), want (nil, nil)", reserved, plain)
	}
}

func TestDeriveSignals_PriorityFieldWins(t *testing.T) {
	// Structured field beats a conflicting label.
	sig := DeriveSignals(Task{Priority: PriorityLow, Labels: []string{"p0"}})
	if sig.Priority != PriorityLow {
		t.Errorf("Priority = %v, want %v (field overrides label)", sig.Priority, PriorityLow)
	}
}

func TestDeriveSignals_PriorityLabelFillsGap(t *testing.T) {
	sig := DeriveSignals(Task{Labels: []string{"p1"}})
	if sig.Priority != PriorityHigh {
		t.Errorf("Priority = %v, want %v (label fills empty field)", sig.Priority, PriorityHigh)
	}
}

func TestDeriveSignals_PriorityMostUrgentLabelWins(t *testing.T) {
	// Multiple priority labels → the most urgent applies.
	sig := DeriveSignals(Task{Labels: []string{"p3", "p1", "p2"}})
	if sig.Priority != PriorityHigh {
		t.Errorf("Priority = %v, want %v (most urgent label)", sig.Priority, PriorityHigh)
	}
}

func TestDeriveSignals_PriorityDefaultsMedium(t *testing.T) {
	sig := DeriveSignals(Task{Labels: []string{"backend"}})
	if sig.Priority != PriorityMedium {
		t.Errorf("Priority = %v, want %v (default floor)", sig.Priority, PriorityMedium)
	}
}

func TestDeriveSignals_NeedsVerify(t *testing.T) {
	if sig := DeriveSignals(Task{Verifier: "claudecode"}); !sig.NeedsVerify {
		t.Error("NeedsVerify should be true when Verifier field is set")
	}
	if sig := DeriveSignals(Task{Labels: []string{"needs-verify"}}); !sig.NeedsVerify {
		t.Error("NeedsVerify should be true when needs-verify label present")
	}
	if sig := DeriveSignals(Task{}); sig.NeedsVerify {
		t.Error("NeedsVerify should be false with neither field nor label")
	}
}

func TestDeriveSignals_ForceBlockedAndAutoMergeable(t *testing.T) {
	sig := DeriveSignals(Task{Labels: []string{"Blocked", "AUTO-MERGEABLE"}}) // case-insensitive
	if !sig.ForceBlocked {
		t.Error("ForceBlocked should be true when blocked label present")
	}
	if !sig.AutoMergeable {
		t.Error("AutoMergeable should be true when auto-mergeable label present")
	}

	clean := DeriveSignals(Task{})
	if clean.ForceBlocked || clean.AutoMergeable {
		t.Errorf("empty task should have no switches set, got %+v", clean)
	}
}
