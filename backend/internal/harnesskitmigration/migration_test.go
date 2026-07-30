package harnesskitmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type migrationFixture struct {
	cfg           Config
	legacy        string
	boundSkill    string
	orphanSkill   string
	boundAgent    string
	orphanAgent   string
	nativeCommand string
	nativeMCP     string
	secret        string
}

func newMigrationFixture(t *testing.T) migrationFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	oneAgentsHome := filepath.Join(root, "oneagents-home")
	legacy := filepath.Join(oneAgentsHome, ".1agents", "skill-manager")
	harnessKitData := filepath.Join(oneAgentsHome, ".1agents", "harnesskit")
	backupRoot := filepath.Join(oneAgentsHome, ".1agents", "migration-backups")

	writeTestFile(t, filepath.Join(legacy, "shared", "alpha", "SKILL.md"), "# Alpha\n")
	writeTestFile(t, filepath.Join(legacy, "shared", "alpha", ".skillmeta.json"), `{"id":"alpha","version":"2","forkedFrom":"base"}`)
	writeTestFile(t, filepath.Join(legacy, "shared", "beta", "SKILL.md"), "# Beta\n")
	writeTestFile(t, filepath.Join(legacy, "agents", "reviewer.md"), "# Reviewer\n")
	writeTestFile(t, filepath.Join(legacy, "agents", "writer.md"), "# Writer\n")
	writeTestFile(t, filepath.Join(legacy, "manifest.json"), `{
  "entries": [
    {"packageDir":"alpha","sourceKind":"git","sourceLocator":"https://example.invalid/a","revision":"abc","forkedFrom":"base"},
    {"packageDir":"beta","sourceKind":"local","sourcePath":"/private/source"}
  ]
}`)
	writeTestFile(t, filepath.Join(legacy, "agents-manifest.json"), `{
  "entries": [
    {"packageDir":"reviewer.md","sourceKind":"git","revision":"def"},
    {"packageDir":"writer.md","sourceKind":"local"}
  ]
}`)
	writeTestFile(t, filepath.Join(legacy, "history", "alpha", "v1", "SKILL.md"), "# Alpha v1\n")
	writeTestFile(t, filepath.Join(legacy, "pending-conflicts", "pending_conflicts.json"), `{"entries":[{"id":"alpha"}]}`)
	writeTestFile(t, filepath.Join(legacy, "slash-commands", "commands", "review.toml"), "name = \"review\"\nprompt = \"Review this\"\n")
	writeTestFile(t, filepath.Join(legacy, "slash-commands", "sync-state.json"), `{
  "commands": {"review": {"claude": {"path":"review.md","contentHash":"abc"}}}
}`)
	secret := "TOP_SECRET_MIGRATION_FIXTURE"
	writeTestFile(t, filepath.Join(legacy, "mcp", "manifest.json"), `{
  "servers": [{"name":"private-server","env":{"TOKEN":"`+secret+`"},"headers":{"Authorization":"Bearer secret"}}]
}`)

	boundSkill := filepath.Join(home, ".agents", "skills", "alpha")
	makeTestSymlink(t, filepath.Join(legacy, "shared", "alpha"), boundSkill)
	boundAgent := filepath.Join(home, ".claude", "agents", "reviewer.md")
	makeTestSymlink(t, filepath.Join(legacy, "agents", "reviewer.md"), boundAgent)
	nativeCommand := filepath.Join(home, ".claude", "commands", "review.md")
	writeTestFile(t, nativeCommand, "# Review\n")
	nativeMCP := filepath.Join(home, ".codex", "config.toml")
	writeTestFile(t, nativeMCP, "[mcp_servers.local]\ncommand = \"local\"\n")

	return migrationFixture{
		cfg: Config{
			Home: home, OneAgentsHome: oneAgentsHome, LegacyDir: legacy,
			HarnessKitDataDir: harnessKitData, BackupRoot: backupRoot,
			Now: func() time.Time {
				return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
			},
		},
		legacy:        legacy,
		boundSkill:    boundSkill,
		orphanSkill:   filepath.Join(home, ".agents", "skills", "beta"),
		boundAgent:    boundAgent,
		orphanAgent:   filepath.Join(home, ".claude", "agents", "writer.md"),
		nativeCommand: nativeCommand,
		nativeMCP:     nativeMCP,
		secret:        secret,
	}
}

func TestBuildPlanIsReadOnlyAndRedactsSecrets(t *testing.T) {
	fixture := newMigrationFixture(t)
	root := filepath.Dir(fixture.cfg.Home)
	before, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	after, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("BuildPlan changed the filesystem")
	}
	if !plan.SourceExists || len(plan.Conflicts) != 0 {
		t.Fatalf("unexpected plan source/conflicts: source=%v conflicts=%+v", plan.SourceExists, plan.Conflicts)
	}
	for _, expected := range []string{
		"skill:materialize", "subagent:materialize",
		"skill:copy-orphan", "subagent:copy-orphan",
		"command:preserve-native", "mcp:preserve-native",
	} {
		if plan.Counts[expected] != 1 {
			t.Errorf("count %q = %d, want 1", expected, plan.Counts[expected])
		}
	}
	if plan.LegacyMetadata.SkillManifestEntries != 2 ||
		plan.LegacyMetadata.AgentManifestEntries != 2 ||
		plan.LegacyMetadata.HistoryPackages != 1 ||
		plan.LegacyMetadata.PendingConflicts != 1 ||
		plan.LegacyMetadata.SlashCommands != 1 ||
		plan.LegacyMetadata.SlashSyncRecords != 1 ||
		plan.LegacyMetadata.MCPServers != 1 {
		t.Fatalf("unexpected legacy metadata summary: %+v", plan.LegacyMetadata)
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(fixture.secret)) || bytes.Contains(payload, []byte("Bearer secret")) {
		t.Fatal("plan leaked an MCP secret")
	}
	for _, lossKind := range []string{
		"skill-source-metadata", "subagent-source-metadata", "skill-history",
		"pending-conflict", "command-canonical-record", "mcp-canonical-record",
	} {
		if !hasLoss(plan.Losses, lossKind) {
			t.Errorf("missing explicit loss report item %q", lossKind)
		}
	}
}

func TestApplyBacksUpMaterializesImportsAndIsIdempotent(t *testing.T) {
	fixture := newMigrationFixture(t)
	var scans int
	fixture.cfg.HKRunner = func(context.Context, Config) error {
		scans++
		return nil
	}
	sourceFingerprint, err := fingerprintTree(fixture.legacy)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Apply(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Materialized != 4 || scans != 1 {
		t.Fatalf("unexpected apply result: result=%+v scans=%d", result, scans)
	}
	for _, path := range []string{fixture.boundSkill, fixture.orphanSkill, fixture.boundAgent, fixture.orphanAgent} {
		assertNativePath(t, path)
	}
	if got, _ := os.ReadFile(filepath.Join(fixture.orphanSkill, "SKILL.md")); string(got) != "# Beta\n" {
		t.Fatalf("orphan skill was not imported: %q", got)
	}
	backupPath := filepath.Join(fixture.cfg.BackupRoot, result.BackupID)
	for _, relative := range []string{
		"legacy-data",
		"legacy-metadata/manifest.json",
		"legacy-metadata/agents-manifest.json",
		"legacy-metadata/history",
		"rewritten-paths",
		"rewritten-paths.json",
		"backup-checksums.json",
		"migration-loss-report.json",
		"migration-record.json",
	} {
		if _, err := os.Lstat(filepath.Join(backupPath, relative)); err != nil {
			t.Errorf("missing backup artifact %s: %v", relative, err)
		}
	}
	lossPayload, err := os.ReadFile(filepath.Join(backupPath, "migration-loss-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(lossPayload, []byte(fixture.secret)) {
		t.Fatal("loss report leaked an MCP secret")
	}
	afterFingerprint, err := fingerprintTree(fixture.legacy)
	if err != nil {
		t.Fatal(err)
	}
	if afterFingerprint != sourceFingerprint {
		t.Fatal("apply modified the legacy data directory")
	}
	if _, err := os.Stat(filepath.Join(fixture.cfg.HarnessKitDataDir, "metadata.db")); !os.IsNotExist(err) {
		t.Fatalf("migration wrote HarnessKit metadata.db directly: %v", err)
	}

	again, err := Apply(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "already-completed" || again.BackupID != result.BackupID || scans != 1 {
		t.Fatalf("apply was not idempotent: result=%+v scans=%d", again, scans)
	}
}

func TestApplyRefusesConflictWithoutChangingNativeTarget(t *testing.T) {
	fixture := newMigrationFixture(t)
	writeTestFile(t, fixture.orphanSkill, "occupied")

	plan, err := BuildPlan(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) == 0 {
		t.Fatal("expected an orphan import collision")
	}
	_, err = Apply(context.Background(), fixture.cfg)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected conflict refusal, got %v", err)
	}
	got, readErr := os.ReadFile(fixture.orphanSkill)
	if readErr != nil || string(got) != "occupied" {
		t.Fatalf("conflicting target changed: content=%q err=%v", got, readErr)
	}
	if entries, readErr := os.ReadDir(fixture.cfg.BackupRoot); readErr == nil && len(entries) != 0 {
		t.Fatalf("backup was created despite a plan conflict: %v", entries)
	}
}

func TestOrphanImportAcceptsExistingIdenticalTarget(t *testing.T) {
	fixture := newMigrationFixture(t)
	if err := copyPath(filepath.Join(fixture.legacy, "shared", "beta"), fixture.orphanSkill); err != nil {
		t.Fatal(err)
	}
	before, err := fingerprintTree(fixture.orphanSkill)
	if err != nil {
		t.Fatal(err)
	}
	var scans int
	fixture.cfg.HKRunner = func(context.Context, Config) error {
		scans++
		return nil
	}

	plan, err := BuildPlan(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 0 || plan.Counts["skill:copy-orphan"] != 0 {
		t.Fatalf("identical native target was not idempotent: %+v", plan)
	}
	if _, err := Apply(context.Background(), fixture.cfg); err != nil {
		t.Fatal(err)
	}
	after, err := fingerprintTree(fixture.orphanSkill)
	if err != nil {
		t.Fatal(err)
	}
	if before != after || scans != 1 {
		t.Fatalf("identical native target changed: before=%s after=%s scans=%d", before, after, scans)
	}
}

func TestApplyResumesAfterScannerFailure(t *testing.T) {
	fixture := newMigrationFixture(t)
	var attempts int
	fixture.cfg.HKRunner = func(context.Context, Config) error {
		attempts++
		if attempts == 1 {
			return errors.New("injected scanner failure")
		}
		return nil
	}

	if _, err := Apply(context.Background(), fixture.cfg); err == nil {
		t.Fatal("expected the injected scanner failure")
	}
	for _, path := range []string{fixture.boundSkill, fixture.orphanSkill, fixture.boundAgent, fixture.orphanAgent} {
		assertNativePath(t, path)
	}
	result, err := Apply(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || attempts != 2 {
		t.Fatalf("migration did not resume: result=%+v attempts=%d", result, attempts)
	}
	entries, err := os.ReadDir(fixture.cfg.BackupRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("resume created %d backups, want 1", len(entries))
	}
}

func TestDataRollbackRestoresLinksAndPreservesDrift(t *testing.T) {
	fixture := newMigrationFixture(t)
	var scans int
	fixture.cfg.HKRunner = func(context.Context, Config) error {
		scans++
		return nil
	}
	result, err := Apply(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(fixture.orphanSkill, "SKILL.md"), "# User changed Beta\n")

	report, err := DataRollback(context.Background(), fixture.cfg, result.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	assertSymlink(t, fixture.boundSkill)
	assertSymlink(t, fixture.boundAgent)
	if _, err := os.Lstat(fixture.orphanAgent); !os.IsNotExist(err) {
		t.Fatalf("unchanged orphan subagent was not removed: %v", err)
	}
	assertNativePath(t, fixture.orphanSkill)
	if !hasConflict(report.Conflicts, fixture.orphanSkill) {
		t.Fatalf("rollback did not report drift: %+v", report)
	}
	if report.Restored != 3 || scans != 2 {
		t.Fatalf("unexpected rollback result: report=%+v scans=%d", report, scans)
	}
	if _, err := os.Stat(filepath.Join(fixture.cfg.BackupRoot, result.BackupID, "harnesskit-post-rollback")); err != nil {
		t.Fatalf("missing post-migration HarnessKit snapshot: %v", err)
	}
}

func TestRollbackRejectsTamperedBackupBeforeChangingTargets(t *testing.T) {
	fixture := newMigrationFixture(t)
	fixture.cfg.HKRunner = func(context.Context, Config) error { return nil }
	result, err := Apply(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(fixture.cfg.BackupRoot, result.BackupID, "legacy-metadata", "manifest.json"), "tampered")

	if _, err := DataRollback(context.Background(), fixture.cfg, result.BackupID); err == nil ||
		!strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected backup checksum refusal, got %v", err)
	}
	assertNativePath(t, fixture.boundSkill)
	assertNativePath(t, fixture.boundAgent)
}

func TestCleanStartDoesNotReadLegacy(t *testing.T) {
	fixture := newMigrationFixture(t)
	fixture.cfg.LegacyDir = filepath.Join(filepath.Dir(fixture.legacy), "does-not-exist")
	var scans int
	fixture.cfg.HKRunner = func(context.Context, Config) error {
		scans++
		return nil
	}

	result, err := CleanStart(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Mode != "clean-start" || scans != 1 {
		t.Fatalf("unexpected clean-start result: result=%+v scans=%d", result, scans)
	}
}

func TestMigrationLockIsExclusive(t *testing.T) {
	fixture := newMigrationFixture(t)
	lock, err := acquireLock(normalizeConfig(fixture.cfg))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	if _, err := Apply(context.Background(), fixture.cfg); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected exclusive lock error, got %v", err)
	}
}

func TestRunCLIPlanWithExplicitRoots(t *testing.T) {
	fixture := newMigrationFixture(t)
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{
		"--plan",
		"--home", fixture.cfg.Home,
		"--oneagents-home", fixture.cfg.OneAgentsHome,
		"--legacy-dir", fixture.cfg.LegacyDir,
		"--harnesskit-data-dir", fixture.cfg.HarnessKitDataDir,
		"--backup-root", fixture.cfg.BackupRoot,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("CLI failed with code %d: %s", code, stderr.String())
	}
	var plan Plan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.SourceExists || len(plan.Conflicts) != 0 {
		t.Fatalf("unexpected CLI plan: %+v", plan)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func makeTestSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}

func assertNativePath(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s is still a symlink", path)
	}
}

func assertSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", path)
	}
}

func hasLoss(losses []Loss, kind string) bool {
	for _, loss := range losses {
		if loss.Kind == kind {
			return true
		}
	}
	return false
}

func hasConflict(conflicts []Conflict, path string) bool {
	for _, conflict := range conflicts {
		if conflict.Path == path {
			return true
		}
	}
	return false
}
