package ccconnect

import (
	"encoding/base64"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/agent"
)

func TestCardActionRoundTrip(t *testing.T) {
	// Workspace paths contain separators; they must round-trip through the
	// single-string callback channel unscathed.
	wsPath := "/Users/scott/projects/my repo:weird"
	taskID := "abc-123-def"

	for _, dec := range []agent.IMDecision{agent.IMApprove, agent.IMReject} {
		val := encodeCardAction(dec, wsPath, taskID)
		gotDec, gotPath, gotID, ok := parseCardAction(val)
		if !ok {
			t.Fatalf("parse %q failed", val)
		}
		if gotDec != dec || gotPath != wsPath || gotID != taskID {
			t.Fatalf("round-trip mismatch: dec=%q path=%q id=%q", gotDec, gotPath, gotID)
		}
	}
}

func TestParseCardActionRejectsBadInput(t *testing.T) {
	cases := []string{
		"cmd:/status",                    // wrong prefix
		"task:bogus:" + b64("p") + ":t",  // unknown decision
		"task:approve:!!!:t",             // bad base64
		"task:approve:" + b64("p") + ":", // empty task id
		"task:approve",                   // too few parts
	}
	for _, c := range cases {
		if _, _, _, ok := parseCardAction(c); ok {
			t.Fatalf("expected parse failure for %q", c)
		}
	}
}

func TestBuildDecisionCardHasButtons(t *testing.T) {
	card := buildDecisionCard(agent.TaskNotification{
		Kind:          agent.NotifyBlocked,
		WorkspacePath: "/ws",
		TaskID:        "t1",
		Number:        7,
		Title:         "受阻任务",
		Summary:       "依赖未满足",
	})
	if !card.HasButtons() {
		t.Fatal("decision card must carry approve/reject buttons")
	}
}

func TestNewTaskNotifierNilDeps(t *testing.T) {
	if newTaskNotifier(nil, nil) != nil {
		t.Fatal("expected nil notifier when bridge/store missing")
	}
}

func b64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
