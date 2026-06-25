package meta

import "strings"

// signals.go turns a task's labels and structured fields into machine-readable
// triggers and policy signals (issue #134). It is the read-only signal layer
// the scheduler — and the future event-orchestration engine (#133) — consume to
// decide flow: who runs, in what order, whether to verify, whether to auto-act
// on completion. This file deliberately ships no orchestration: it only derives
// a normalized snapshot from data already on the task.
//
// The contract: structured fields are the source of truth; reserved labels are
// a lightweight, GitHub-style way to express the same intent for tasks that came
// in as bare label sets (imports, IM quick-adds). A reserved label is only
// honored when the corresponding structured field is unset, so explicit fields
// always win and the two can't silently disagree.

// Reserved labels carry policy meaning. Anything not listed here is a plain
// human-facing tag with no effect on scheduling/orchestration.
const (
	// Priority overrides (GitHub p0..p3 convention). Lower number = more urgent.
	LabelP0 = "p0" // → urgent
	LabelP1 = "p1" // → high
	LabelP2 = "p2" // → medium
	LabelP3 = "p3" // → low

	// LabelBlocked force-holds a task out of the runnable queue regardless of
	// its dependency graph — an explicit "do not run yet" switch.
	LabelBlocked = "blocked"

	// LabelNeedsVerify requests an adversarial verification pass after the
	// executor finishes (mirrors the Verifier field; see #50/#135).
	LabelNeedsVerify = "needs-verify"

	// LabelAutoMergeable authorizes downstream automation to merge/close the
	// produced artifact once verification passes — a policy grant, not an action
	// (#133 owns the action).
	LabelAutoMergeable = "auto-mergeable"
)

// reservedLabels is the lookup set used by IsReservedLabel / SplitLabels.
var reservedLabels = map[string]struct{}{
	LabelP0:            {},
	LabelP1:            {},
	LabelP2:            {},
	LabelP3:            {},
	LabelBlocked:       {},
	LabelNeedsVerify:   {},
	LabelAutoMergeable: {},
}

// PolicySignals is the normalized, machine-readable view of a task's triggers
// and policy switches — derived, never persisted. Consumers read it instead of
// re-parsing labels/fields so the precedence rules live in exactly one place.
type PolicySignals struct {
	// Priority is the effective scheduling priority after applying any p0..p3
	// label override. Always one of urgent/high/medium/low.
	Priority Priority `json:"priority"`
	// ForceBlocked is true when a `blocked` label explicitly holds the task,
	// independent of its dependency graph.
	ForceBlocked bool `json:"forceBlocked"`
	// NeedsVerify is true when the task requests a verification pass — either
	// via the Verifier field or the `needs-verify` label.
	NeedsVerify bool `json:"needsVerify"`
	// AutoMergeable is true when downstream automation is authorized to
	// merge/close the artifact on success (the `auto-mergeable` label).
	AutoMergeable bool `json:"autoMergeable"`
}

// labelToPriority maps a reserved priority label to its Priority, or "" if the
// label is not a priority label.
func labelToPriority(label string) Priority {
	switch label {
	case LabelP0:
		return PriorityUrgent
	case LabelP1:
		return PriorityHigh
	case LabelP2:
		return PriorityMedium
	case LabelP3:
		return PriorityLow
	default:
		return ""
	}
}

// IsReservedLabel reports whether a label has policy meaning (case-insensitive,
// surrounding whitespace ignored). Reserved labels are normalized to lowercase
// for the lookup so "P0" and "p0" mean the same thing.
func IsReservedLabel(label string) bool {
	_, ok := reservedLabels[strings.ToLower(strings.TrimSpace(label))]
	return ok
}

// SplitLabels partitions a task's labels into the reserved (policy-bearing) set
// and the plain human-facing set, preserving order and original casing. It is
// the validation/inspection entry point: callers can show users which of their
// labels actually drive automation. Reserved labels are de-duplicated (by
// normalized form); plain labels are returned as-is.
func SplitLabels(labels []string) (reserved, plain []string) {
	seen := make(map[string]struct{})
	for _, l := range labels {
		if IsReservedLabel(l) {
			key := strings.ToLower(strings.TrimSpace(l))
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			reserved = append(reserved, l)
		} else {
			plain = append(plain, l)
		}
	}
	return reserved, plain
}

// DeriveSignals computes the policy signals for a task. Precedence rule:
// structured fields are authoritative; a reserved label only fills a gap the
// field left open, so explicit fields can never be overridden by a stray label.
//
//   - Priority: Task.Priority wins; if empty, the most urgent p0..p3 label
//     present applies; if none, medium (PriorityRank's default).
//   - NeedsVerify: true if Task.Verifier is set OR a needs-verify label present.
//   - ForceBlocked: true iff a blocked label is present (no structured analog;
//     the scheduler's derived `blocked` status is about dependencies, this is an
//     explicit manual hold).
//   - AutoMergeable: true iff an auto-mergeable label is present.
func DeriveSignals(t Task) PolicySignals {
	sig := PolicySignals{
		Priority:    t.Priority,
		NeedsVerify: strings.TrimSpace(t.Verifier) != "",
	}

	// Scan labels once, collecting the most-urgent priority override and the
	// boolean switches.
	var labelPriority Priority
	for _, l := range t.Labels {
		norm := strings.ToLower(strings.TrimSpace(l))
		switch norm {
		case LabelBlocked:
			sig.ForceBlocked = true
		case LabelNeedsVerify:
			sig.NeedsVerify = true
		case LabelAutoMergeable:
			sig.AutoMergeable = true
		default:
			if p := labelToPriority(norm); p != "" {
				if labelPriority == "" || PriorityRank(p) < PriorityRank(labelPriority) {
					labelPriority = p
				}
			}
		}
	}

	// Field wins; label fills the gap; medium is the floor.
	if sig.Priority == "" {
		if labelPriority != "" {
			sig.Priority = labelPriority
		} else {
			sig.Priority = PriorityMedium
		}
	}

	return sig
}
