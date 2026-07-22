package agent

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// events.go is the event-driven orchestration engine (#133). It borrows
// GitHub Actions' `on:event → run` model: instead of a human watching the
// board and hand-dispatching work, lifecycle transitions emit events and a
// declarative rule table reacts — routing, notifying, requeueing.
//
// Design boundary: the engine never re-implements scheduling. The scheduler
// (scheduler.go) remains the single owner of state transitions and the
// workspace lock; it is the only place that detects a transition. At each
// transition point it emits a TaskEvent. Rules are pure functions
// (event → []EventAction) that decide *what* should happen; the scheduler
// applies the returned actions to the live task inside its existing Mutate
// transaction. This keeps "what to do" (declarative, here) cleanly separate
// from "how to apply it" (the scheduler's locking/persistence), and makes the
// whole rule layer unit-testable with no runner or bridge attached.
//
// The engine consumes the #134 policy-signal layer (PolicySignals) for routing
// decisions rather than re-parsing labels/fields.

// TaskEventKind is a lifecycle event in a task's life. These are the six
// acceptance-criteria events of #133.
type TaskEventKind string

const (
	// EventCreated fires once when the scheduler first observes a fresh,
	// unassigned executable task entering the runnable pipeline.
	EventCreated TaskEventKind = "created"
	// EventAssigned fires when a task gains an executing agent (Assignee) —
	// typically as the result of a created→route rule.
	EventAssigned TaskEventKind = "assigned"
	// EventBlocked fires when a task transitions into the blocked status
	// (unmet dependency or an explicit `blocked` hold).
	EventBlocked TaskEventKind = "blocked"
	// EventVerifyFailed fires when a verification pass rejects the artifact
	// but the review budget still has room — the task will be re-executed.
	EventVerifyFailed TaskEventKind = "verify-failed"
	// EventVerifyNeedsHuman fires when a verifier judges the artifact needs a
	// human decision (design/architecture/tradeoff), not another executor round.
	// The task escalates to awaiting_human instead of consuming review budget.
	EventVerifyNeedsHuman TaskEventKind = "verify-needs-human"
	// EventFailed fires when a task reaches the terminal failed status.
	EventFailed TaskEventKind = "failed"
	// EventDone fires when a task reaches the terminal completed status.
	EventDone TaskEventKind = "done"
)

// TaskEvent is an immutable record of one lifecycle transition. Task is a
// snapshot (by value) so a rule can read it freely without racing the
// scheduler's live slice. Signals is the derived #134 view, precomputed so
// every rule sees identical routing inputs.
type TaskEvent struct {
	Kind          TaskEventKind
	Task          Task
	Signals       PolicySignals
	WorkspacePath string
	At            time.Time
}

// EventActionKind is the verb of an action a rule asks the scheduler to apply.
type EventActionKind string

const (
	// ActionAssign sets the task's executing agent (Assignee) to AgentType.
	// No-op if the task already has that assignee.
	ActionAssign EventActionKind = "assign"
	// ActionNotify appends a notification comment to the task timeline
	// addressed to a role (e.g. the PM). The board/IM surfaces pick it up
	// like any other reply.
	ActionNotify EventActionKind = "notify"
	// ActionRequeue forces a task back to pending so the scheduler re-runs it,
	// carrying Note as context. Used by the verify-failed chain.
	ActionRequeue EventActionKind = "requeue"
	// ActionAwaitHuman parks a task at awaiting_human, carrying Note as the
	// escalation reason. Used by the verify-needs-human chain: the task waits
	// for a human decision (complete_human_project_item / board completion) rather than
	// being re-executed. Downstream deps stay blocked until it completes.
	ActionAwaitHuman EventActionKind = "await-human"
)

// EventAction is the declarative result of a rule: a verb plus its target.
// It is plain data — the scheduler interprets and applies it, the rule never
// touches the store.
type EventAction struct {
	Kind EventActionKind
	// AgentType is the target executor for ActionAssign.
	AgentType string
	// Role is the addressee for ActionNotify (e.g. "pm").
	Role string
	// Note is the human-readable reason carried into a notify/requeue comment.
	Note string
}

// Rule is one declarative `on:event → action` entry. On lists the event kinds
// the rule reacts to; Do computes the actions for a matching event (return nil
// to do nothing). Rules are pure: same event in, same actions out, no I/O.
type Rule struct {
	Name string
	On   []TaskEventKind
	Do   func(ev TaskEvent) []EventAction
}

// matches reports whether this rule reacts to the event's kind.
func (r Rule) matches(kind TaskEventKind) bool {
	for _, k := range r.On {
		if k == kind {
			return true
		}
	}
	return false
}

// EventEngine holds the rule table and evaluates events against it. It carries
// no scheduler/store references on purpose — the scheduler drives it and
// applies the results — so it can be exercised in isolation.
type EventEngine struct {
	rules []Rule
}

// NewEventEngine builds an engine over the given rules.
func NewEventEngine(rules []Rule) *EventEngine {
	return &EventEngine{rules: rules}
}

// DefaultEventEngine wires the built-in rule table that satisfies #133's two
// required chains plus the PM-notify on block. The table is declarative data;
// adding a chain means adding a Rule here, not editing the scheduler.
func DefaultEventEngine() *EventEngine {
	return NewEventEngine(defaultRules())
}

// Evaluate runs every matching rule and returns the concatenated actions, in
// rule-declaration order. Deterministic: no map iteration, no goroutines.
func (e *EventEngine) Evaluate(ev TaskEvent) []EventAction {
	if e == nil {
		return nil
	}
	var out []EventAction
	for _, r := range e.rules {
		if !r.matches(ev.Kind) {
			continue
		}
		out = append(out, r.Do(ev)...)
	}
	return out
}

// defaultRules is the shipped `on:task-* → action` table.
//
//   - created → route: an executable task with no assignee is routed to an
//     agent picked by its type/domain (routeAgent). This is the
//     `on:task-created → 按类型/领域派对应 agent` chain.
//   - blocked → notify PM: a freshly blocked task pings the PM so a human can
//     unblock it (`on:task-blocked → 通知 PM`).
//   - verify-failed → requeue executor with the failure context
//     (`on:verify-failed → 自动重派执行者(带失败上下文)`). The scheduler/review
//     loop already moves the task back to pending; this rule annotates the
//     requeue with the rejection summary so the next run carries it.
func defaultRules() []Rule {
	return []Rule{
		{
			Name: "route-on-created",
			On:   []TaskEventKind{EventCreated},
			Do: func(ev TaskEvent) []EventAction {
				if strings.TrimSpace(ev.Task.Assignee) != "" {
					return nil // already assigned — nothing to route
				}
				return []EventAction{{
					Kind:      ActionAssign,
					AgentType: routeAgent(ev.Task, ev.Signals),
					Note:      "自动派单",
				}}
			},
		},
		{
			Name: "notify-pm-on-blocked",
			On:   []TaskEventKind{EventBlocked},
			Do: func(ev TaskEvent) []EventAction {
				reason := "依赖未满足"
				if ev.Signals.ForceBlocked {
					reason = "被显式标记 blocked"
				}
				return []EventAction{{
					Kind: ActionNotify,
					Role: SessionRolePM,
					Note: "任务被阻塞(" + reason + "),需要 PM 介入解阻。",
				}}
			},
		},
		{
			Name: "requeue-on-verify-failed",
			On:   []TaskEventKind{EventVerifyFailed},
			Do: func(ev TaskEvent) []EventAction {
				note := "核验未通过,自动重派执行者重做。"
				if ev.Task.Review != nil && strings.TrimSpace(ev.Task.Review.Summary) != "" {
					note = "核验未通过,自动重派执行者重做。失败原因:" + ev.Task.Review.Summary
				}
				return []EventAction{{Kind: ActionRequeue, Note: note}}
			},
		},
		{
			Name: "escalate-on-verify-needs-human",
			On:   []TaskEventKind{EventVerifyNeedsHuman},
			Do: func(ev TaskEvent) []EventAction {
				note := "核验判定需人工介入,已升级等待人工决策。"
				if ev.Task.Review != nil && strings.TrimSpace(ev.Task.Review.Summary) != "" {
					note = "核验判定需人工介入,已升级等待人工决策。原因:" + ev.Task.Review.Summary
				}
				return []EventAction{{Kind: ActionAwaitHuman, Role: SessionRolePM, Note: note}}
			},
		},
	}
}

// routeAgent picks the executing agent for a freshly created task by type and
// domain labels (#133: 按类型/领域派对应 agent). Deliberately small — a lookup
// table, not a planner. Falls back to DefaultAgentType. Domain labels win over
// type because they are the more specific signal.
func routeAgent(t Task, _ PolicySignals) AgentType {
	for _, l := range t.Labels {
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "frontend", "前端", "ui":
			return AgentTypeClaudecode
		case "research", "调研", "analysis":
			return AgentTypeGemini
		}
	}
	switch t.Type {
	case ItemTypeBug:
		return AgentTypeCodex
	default:
		return DefaultAgentType
	}
}

// applyEventActions applies a rule engine's actions to a live task pointer
// inside the scheduler's Mutate transaction. The task pointer is part of
// cfg.Tasks, so mutations persist when Mutate saves. It returns whether
// anything was modified plus the follow-on events the actions produced (e.g.
// an assign action emits EventAssigned) so the caller can fan those back
// through the engine within the same tick.
func applyEventActions(t *Task, actions []EventAction, now time.Time) (modified bool, followUps []TaskEventKind) {
	for _, a := range actions {
		switch a.Kind {
		case ActionAssign:
			if a.AgentType == "" || t.Assignee == a.AgentType {
				continue
			}
			t.Assignee = a.AgentType
			t.UpdatedAt = now
			modified = true
			followUps = append(followUps, EventAssigned)
			log.Printf("[events] task %s routed to %s (%s)", t.ID, a.AgentType, a.Note)
		case ActionNotify:
			t.Replies = append(t.Replies, Reply{
				Author:    Author{Kind: "scheduler", Name: "scheduler"},
				Text:      fmt.Sprintf("@%s %s", a.Role, a.Note),
				Mode:      ModePureComment,
				CreatedAt: now,
			})
			t.UpdatedAt = now
			modified = true
			log.Printf("[events] task %s notify %s: %s", t.ID, a.Role, a.Note)
		case ActionRequeue:
			t.Status = TaskStatusPending
			t.StartedAt = nil
			t.UpdatedAt = now
			if strings.TrimSpace(a.Note) != "" {
				t.Replies = append(t.Replies, Reply{
					Author:    Author{Kind: "scheduler", Name: "scheduler"},
					Text:      a.Note,
					Mode:      ModePureComment,
					CreatedAt: now,
				})
			}
			modified = true
			log.Printf("[events] task %s requeued by rule (%s)", t.ID, a.Note)
		case ActionAwaitHuman:
			// Park for a human decision. Keep StartedAt (the task already ran +
			// verified) so its elapsed time reflects the real work. The
			// scheduler skips awaiting_human, so this holds until a human
			// completes it via complete_human_project_item / board completion.
			t.Status = TaskStatusAwaitingHuman
			t.UpdatedAt = now
			if strings.TrimSpace(a.Note) != "" {
				text := a.Note
				if strings.TrimSpace(a.Role) != "" {
					text = fmt.Sprintf("@%s %s", a.Role, a.Note)
				}
				t.Replies = append(t.Replies, Reply{
					Author:    Author{Kind: "scheduler", Name: "scheduler"},
					Text:      text,
					Mode:      ModePureComment,
					CreatedAt: now,
				})
			}
			modified = true
			log.Printf("[events] task %s escalated to awaiting_human by rule (%s)", t.ID, a.Note)
		}
	}
	return modified, followUps
}
