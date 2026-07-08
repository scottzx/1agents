package govern

import (
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// TestBuiltinSteps_ShapeAndRun verifies the built-in governors are exposed as
// well-formed steps and RunBuiltin drives every one, recording a log entry each.
// On empty stores every governor is a no-op (0 rows, success) — proving the run
// path is wired without needing real bronze data.
func TestBuiltinSteps_ShapeAndRun(t *testing.T) {
	steps := BuiltinSteps()
	if len(steps) == 0 {
		t.Fatal("BuiltinSteps() empty")
	}
	names := map[string]bool{}
	for _, s := range steps {
		if s.Name == "" || s.Run == nil {
			t.Fatalf("malformed step: %+v", s)
		}
		if s.Tier != TierSilver && s.Tier != TierGold {
			t.Fatalf("step %s bad tier %q", s.Name, s.Tier)
		}
		if names[s.Name] {
			t.Fatalf("duplicate step name %q", s.Name)
		}
		names[s.Name] = true
	}
	// Silver must precede gold in run order (gold reads silver).
	sawGold := false
	for _, s := range steps {
		if s.Tier == TierGold {
			sawGold = true
		}
		if s.Tier == TierSilver && sawGold {
			t.Fatalf("silver step %s ordered after a gold step", s.Name)
		}
	}

	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	bronze, err := sources.OpenDefault()
	if err != nil {
		t.Fatalf("bronze: %v", err)
	}
	dst, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("data: %v", err)
	}

	var logged []RunRecord
	rec := func(r RunRecord) { logged = append(logged, r) }
	if err := RunBuiltin(bronze, dst, rec); err != nil {
		t.Fatalf("RunBuiltin: %v", err)
	}
	if len(logged) != len(steps) {
		t.Fatalf("recorded %d runs, want %d", len(logged), len(steps))
	}
	for _, r := range logged {
		if r.Status != "success" || r.Lang != "go" {
			t.Fatalf("run %s: status=%s lang=%s", r.Step, r.Status, r.Lang)
		}
	}

	// Single-step re-run resolves by name; an unknown name is reported not-found.
	if _, found, err := RunBuiltinStep(bronze, dst, "silver_feishu_users", rec); err != nil || !found {
		t.Fatalf("RunBuiltinStep known: found=%v err=%v", found, err)
	}
	if _, found, _ := RunBuiltinStep(bronze, dst, "does_not_exist", rec); found {
		t.Fatal("RunBuiltinStep should not find a bogus step")
	}
}
