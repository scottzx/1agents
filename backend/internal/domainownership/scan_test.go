package domainownership

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// backendRoot returns the absolute path of the backend module root
// (the directory holding internal/ and cmd/), derived from this test file's
// location so the gate works regardless of the test runner's cwd.
func backendRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	// .../backend/internal/domainownership/scan_test.go → backend root.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestScanFixtureCatchesViolations(t *testing.T) {
	root := filepath.Join(backendRoot(t), "internal", "domainownership", "testdata", "fixture")
	violations, err := ScanBackend(root)
	if err != nil {
		t.Fatalf("ScanBackend(fixture): %v", err)
	}

	counts := map[Rule]int{}
	for _, v := range violations {
		counts[v.Rule]++
	}

	want := map[Rule]int{
		RuleAppImportsApp:       1, // beta → alpha implementation
		RuleForeignRepoImport:   1, // beta → alpha repository
		RuleAppImportsNonSDK:    1, // beta → internal/meta
		RuleCrossDomainSQLWrite: 4, // beta→alpha_leads, beta→projects, beta→presales_, kernel→presales_
		RuleCrossDomainSQLRead:  1, // beta reads alpha_leads
	}
	for rule, n := range want {
		if counts[rule] != n {
			t.Errorf("rule %s: got %d violations, want %d\nall: %s", rule, counts[rule], n, violations)
		}
	}

	// Legal files must never be flagged.
	for _, v := range violations {
		for _, legal := range []string{
			"internal/apps/apps.go",
			"internal/apps/alpha/register.go",
			"internal/apps/alpha/repository/repository.go",
			"internal/kernel/good_writer.go",
			"cmd/agent/main.go",
		} {
			if strings.Contains(filepath.ToSlash(v.File), legal) {
				t.Errorf("legal file %s flagged: %s", legal, v)
			}
		}
	}
}

// TestRealTreeIsClean is the production gate: the actual backend source must
// contain no ownership violations. It runs in `go test ./...` and CI, so any
// new cross-domain write or app-to-app import fails the build.
func TestRealTreeIsClean(t *testing.T) {
	root := backendRoot(t)
	violations, err := ScanBackend(root)
	if err != nil {
		t.Fatalf("ScanBackend(real): %v", err)
	}
	if len(violations) > 0 {
		var b strings.Builder
		for _, v := range violations {
			b.WriteString("\n  " + v.String())
		}
		t.Fatalf("architecture gate found %d violation(s) in the backend tree:%s", len(violations), b.String())
	}
}

// TestScannerExercisesKernelDDL guards against a vacuous pass: meta/db.go is
// full of CREATE TABLE statements, so the scanner must parse and classify
// them (and find them legal kernel writes) rather than skip the file.
func TestScannerExercisesKernelDDL(t *testing.T) {
	root := backendRoot(t)
	violations := scanFile("internal/meta/db.go", filepath.Join(root, "internal", "meta", "db.go"), nil)
	for _, v := range violations {
		if v.Rule == RuleParseError {
			t.Fatalf("meta/db.go failed to parse: %s", v)
		}
		t.Errorf("unexpected violation in meta/db.go: %s", v)
	}
	// And a deliberately cross-domain kernel write must be flagged.
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.go")
	content := "package kernel\n\nconst q = `INSERT INTO commerce_products (id) VALUES ('x')`\n"
	if err := writeFile(bad, content); err != nil {
		t.Fatal(err)
	}
	got := scanFile("internal/kernel/bad.go", bad, nil)
	if len(got) != 1 || got[0].Rule != RuleCrossDomainSQLWrite {
		t.Fatalf("expected one cross_domain_sql_write, got %v", got)
	}
}

func TestAppNamespaceOf(t *testing.T) {
	cases := []struct {
		rel   string
		ns    string
		isApp bool
	}{
		{"internal/apps/presales/store.go", "presales", true},
		{"internal/apps/presales/repository/r.go", "presales", true},
		{"internal/apps/apps.go", NamespaceKernel, false}, // aggregator
		{"internal/meta/db.go", NamespaceKernel, false},
		{"cmd/backend/main.go", NamespaceKernel, false},
	}
	for _, c := range cases {
		ns, isApp := appNamespaceOf(c.rel)
		if ns != c.ns || isApp != c.isApp {
			t.Errorf("appNamespaceOf(%q) = (%q,%v), want (%q,%v)", c.rel, ns, isApp, c.ns, c.isApp)
		}
	}
}
