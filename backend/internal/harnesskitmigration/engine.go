package harnesskitmigration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type inProgressRecord struct {
	OperationID string `json:"operationId"`
	JournalPath string `json:"journalPath"`
}

func Apply(ctx context.Context, cfg Config) (Result, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfigPaths(cfg); err != nil {
		return Result{}, err
	}
	lock, err := acquireLock(cfg)
	if err != nil {
		return Result{}, err
	}
	defer lock.release()
	if err := ensureServicesStopped(cfg, true); err != nil {
		return Result{}, err
	}

	plan, err := BuildPlan(cfg)
	if err != nil {
		return Result{}, err
	}
	if !plan.SourceExists {
		return Result{}, fmt.Errorf("legacy Skills-manager data not found at %s; use --clean-start for a new installation", cfg.LegacyDir)
	}
	if len(plan.Conflicts) > 0 {
		return Result{}, fmt.Errorf("migration plan has %d conflict(s); no files were changed", len(plan.Conflicts))
	}
	markerPath := filepath.Join(cfg.HarnessKitDataDir, "migrations", markerFileName)
	var marker Marker
	if err := readJSON(markerPath, &marker); err == nil && marker.RolledBackAt == nil {
		if marker.SourceFingerprint == plan.SourceFingerprint && marker.Mode == "apply" {
			_ = os.Remove(filepath.Join(cfg.HarnessKitDataDir, "migrations", inProgressFileName))
			return Result{
				Status: "already-completed", Mode: "apply", BackupID: marker.BackupID,
				SourceFingerprint: plan.SourceFingerprint, MarkerPath: markerPath,
			}, nil
		}
		return Result{}, fmt.Errorf("migration already completed for a different legacy fingerprint; refusing implicit incremental import")
	}

	journalPath, journal, err := loadOrCreateJournal(cfg, plan)
	if err != nil {
		return Result{}, err
	}
	if journal.SourceFingerprint != plan.SourceFingerprint {
		return Result{}, fmt.Errorf("legacy source changed after migration preparation; remove no files and regenerate the plan")
	}
	if err := executeApply(ctx, cfg, journalPath, &journal); err != nil {
		journal.Phase = "failed"
		journal.Error = publicOperationError(err)
		journal.UpdatedAt = cfg.Now().UTC()
		_ = atomicWriteJSON(journalPath, journal, 0o600)
		return Result{}, err
	}

	result := Result{
		Status:            "completed",
		Mode:              "apply",
		BackupID:          journal.BackupID,
		SourceFingerprint: journal.SourceFingerprint,
		LossReportPath:    filepath.Join(journal.BackupPath, "migration-loss-report.json"),
		MarkerPath:        markerPath,
	}
	for _, item := range journal.Items {
		switch item.Status {
		case "materialized":
			result.Materialized++
		case "unchanged":
			result.Unchanged++
		}
	}
	return result, nil
}

func loadOrCreateJournal(cfg Config, plan Plan) (string, Journal, error) {
	migrationsDir := filepath.Join(cfg.HarnessKitDataDir, "migrations")
	inProgressPath := filepath.Join(migrationsDir, inProgressFileName)
	var pointer inProgressRecord
	if err := readJSON(inProgressPath, &pointer); err == nil {
		expectedJournalPath := filepath.Join(migrationsDir, pointer.OperationID, "operation-journal.json")
		if pointer.OperationID == "" || pointer.JournalPath != expectedJournalPath {
			return "", Journal{}, fmt.Errorf("invalid incomplete migration pointer")
		}
		var journal Journal
		if err := readJSON(pointer.JournalPath, &journal); err != nil {
			return "", Journal{}, fmt.Errorf("read incomplete migration journal: %w", err)
		}
		if err := validateJournal(cfg, journal); err != nil {
			return "", Journal{}, fmt.Errorf("validate incomplete migration journal: %w", err)
		}
		return pointer.JournalPath, journal, nil
	}

	operationID := operationIdentifier(cfg.Now())
	backupID := "skills-manager-" + operationID
	backupPath := filepath.Join(cfg.BackupRoot, backupID)
	journalPath := filepath.Join(migrationsDir, operationID, "operation-journal.json")
	items := make([]JournalItem, 0)
	for _, item := range plan.Items {
		if item.Action == "materialize" || item.Action == "copy-orphan" {
			items = append(items, JournalItem{Item: item, Status: "pending"})
		}
	}
	now := cfg.Now().UTC()
	journal := Journal{
		Version: planVersion, OperationID: operationID, Phase: "prepared",
		StartedAt: now, UpdatedAt: now, BackupID: backupID, BackupPath: backupPath,
		SourceFingerprint: plan.SourceFingerprint, Plan: plan, Items: items,
	}
	if err := atomicWriteJSON(journalPath, journal, 0o600); err != nil {
		return "", Journal{}, err
	}
	if err := atomicWriteJSON(inProgressPath, inProgressRecord{
		OperationID: operationID, JournalPath: journalPath,
	}, 0o600); err != nil {
		return "", Journal{}, err
	}
	return journalPath, journal, nil
}

func executeApply(ctx context.Context, cfg Config, journalPath string, journal *Journal) error {
	if err := validateJournal(cfg, *journal); err != nil {
		return err
	}
	if current, err := fingerprintTree(cfg.LegacyDir); err != nil || current != journal.SourceFingerprint {
		if err != nil {
			return fmt.Errorf("recheck legacy source: %w", err)
		}
		return errors.New("legacy source fingerprint changed before apply")
	}
	if err := preflightMaterializations(journal.OperationID, journal.Items); err != nil {
		return err
	}

	if !journal.BackupComplete {
		if !pathWithin(journal.BackupPath, cfg.BackupRoot) || journal.BackupPath == cfg.BackupRoot {
			return fmt.Errorf("unsafe backup path: %s", journal.BackupPath)
		}
		if _, err := os.Stat(journal.BackupPath); err == nil {
			if err := os.RemoveAll(journal.BackupPath); err != nil {
				return fmt.Errorf("remove incomplete backup: %w", err)
			}
		}
		if err := os.MkdirAll(journal.BackupPath, 0o700); err != nil {
			return err
		}
		if err := copyPath(cfg.LegacyDir, filepath.Join(journal.BackupPath, "legacy-data")); err != nil {
			return fmt.Errorf("backup legacy data: %w", err)
		}
		if err := backupLegacyMetadata(cfg.LegacyDir, filepath.Join(journal.BackupPath, "legacy-metadata")); err != nil {
			return fmt.Errorf("backup legacy metadata: %w", err)
		}
		if err := backupRewrittenPaths(journal.Items, filepath.Join(journal.BackupPath, "rewritten-paths")); err != nil {
			return fmt.Errorf("backup rewritten paths: %w", err)
		}
		if err := atomicWriteJSON(filepath.Join(journal.BackupPath, "rewritten-paths.json"), journal.Items, 0o600); err != nil {
			return err
		}
		if err := atomicWriteJSON(filepath.Join(journal.BackupPath, "migration-loss-report.json"), journal.Plan.Losses, 0o600); err != nil {
			return err
		}
		if err := writeBackupChecksums(journal.BackupPath); err != nil {
			return err
		}
		journal.BackupComplete = true
		journal.Phase = "backed-up"
		journal.UpdatedAt = cfg.Now().UTC()
		if err := atomicWriteJSON(journalPath, journal, 0o600); err != nil {
			return err
		}
	} else if err := verifyBackupChecksums(journal.BackupPath); err != nil {
		return fmt.Errorf("verify completed backup: %w", err)
	}

	journal.Phase = "materializing"
	journal.UpdatedAt = cfg.Now().UTC()
	if err := atomicWriteJSON(journalPath, journal, 0o600); err != nil {
		return err
	}
	for index := range journal.Items {
		if journal.Items[index].Status == "materialized" || journal.Items[index].Status == "unchanged" {
			continue
		}
		status, postFingerprint, err := applyItem(journal.OperationID, journal.Items[index].Item)
		if err != nil {
			journal.Items[index].Status = "failed"
			journal.Items[index].Error = publicOperationError(err)
			journal.UpdatedAt = cfg.Now().UTC()
			_ = atomicWriteJSON(journalPath, journal, 0o600)
			return err
		}
		journal.Items[index].Status = status
		journal.Items[index].PostFingerprint = postFingerprint
		journal.Items[index].Error = ""
		journal.UpdatedAt = cfg.Now().UTC()
		if err := atomicWriteJSON(journalPath, journal, 0o600); err != nil {
			return err
		}
	}

	journal.Phase = "scanning"
	journal.UpdatedAt = cfg.Now().UTC()
	if err := atomicWriteJSON(journalPath, journal, 0o600); err != nil {
		return err
	}
	if err := cfg.HKRunner(ctx, cfg); err != nil {
		return err
	}

	now := cfg.Now().UTC()
	journal.Phase = "committed"
	journal.Error = ""
	journal.CompletedAt = &now
	journal.UpdatedAt = now
	if err := atomicWriteJSON(journalPath, journal, 0o600); err != nil {
		return err
	}
	if err := atomicWriteJSON(filepath.Join(journal.BackupPath, "migration-record.json"), journal, 0o600); err != nil {
		return err
	}
	markerPath := filepath.Join(cfg.HarnessKitDataDir, "migrations", markerFileName)
	if err := atomicWriteJSON(markerPath, Marker{
		Version: planVersion, MigrationID: migrationID, Mode: "apply",
		SourceFingerprint: journal.SourceFingerprint, BackupID: journal.BackupID, CompletedAt: now,
	}, 0o600); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(cfg.HarnessKitDataDir, "migrations", inProgressFileName))
	return nil
}

func preflightMaterializations(operationID string, items []JournalItem) error {
	for _, journalItem := range items {
		item := journalItem.Item
		info, err := os.Lstat(item.Path)
		if err != nil {
			if os.IsNotExist(err) {
				tempPath := materializationTempPath(item.Path, operationID)
				if _, tempErr := os.Lstat(tempPath); tempErr == nil {
					fingerprint, hashErr := fingerprintTree(tempPath)
					if hashErr != nil || fingerprint != item.Fingerprint {
						return fmt.Errorf("staged content mismatch at %s", tempPath)
					}
				} else if !os.IsNotExist(tempErr) {
					return tempErr
				}
				continue
			}
			return err
		}
		if item.Action == "copy-orphan" {
			fingerprint, hashErr := bindingFingerprint(item.Path, info)
			if hashErr != nil || fingerprint != item.Fingerprint {
				return fmt.Errorf("target collision at %s; refusing overwrite", item.Path)
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(item.Path)
			if err != nil {
				return err
			}
			if target != item.LinkTarget {
				return fmt.Errorf("symlink target changed at %s", item.Path)
			}
			_, fingerprint, err := fingerprintResolved(item.Path)
			if err != nil || fingerprint != item.Fingerprint {
				return fmt.Errorf("symlink source changed at %s", item.Path)
			}
			continue
		}
		fingerprint, err := fingerprintTree(item.Path)
		if err != nil || fingerprint != item.Fingerprint {
			return fmt.Errorf("target collision at %s; refusing overwrite", item.Path)
		}
	}
	return nil
}

func applyItem(operationID string, item Item) (string, string, error) {
	info, err := os.Lstat(item.Path)
	if item.Action == "copy-orphan" && err == nil {
		fingerprint, hashErr := bindingFingerprint(item.Path, info)
		if hashErr != nil {
			return "", "", hashErr
		}
		if fingerprint != item.Fingerprint {
			return "", "", fmt.Errorf("native target drifted at %s", item.Path)
		}
		return "unchanged", fingerprint, nil
	}
	if err == nil && info.Mode()&os.ModeSymlink == 0 {
		fingerprint, hashErr := fingerprintTree(item.Path)
		if hashErr != nil {
			return "", "", hashErr
		}
		if fingerprint != item.Fingerprint {
			return "", "", fmt.Errorf("native target drifted at %s", item.Path)
		}
		return "unchanged", fingerprint, nil
	}
	tempPath := materializationTempPath(item.Path, operationID)
	if os.IsNotExist(err) {
		if _, tempErr := os.Lstat(tempPath); os.IsNotExist(tempErr) {
			if item.Action != "copy-orphan" {
				return "", "", fmt.Errorf("target disappeared before materialization: %s", item.Path)
			}
			if err := copyPath(item.SourcePath, tempPath); err != nil {
				return "", "", err
			}
		} else if tempErr != nil {
			return "", "", tempErr
		}
	} else if err != nil {
		return "", "", err
	} else {
		linkTarget, readErr := os.Readlink(item.Path)
		if readErr != nil || linkTarget != item.LinkTarget {
			return "", "", fmt.Errorf("symlink changed before materialization: %s", item.Path)
		}
		if _, tempErr := os.Lstat(tempPath); os.IsNotExist(tempErr) {
			if err := copyPath(item.SourcePath, tempPath); err != nil {
				return "", "", err
			}
		}
	}
	tempFingerprint, err := fingerprintTree(tempPath)
	if err != nil || tempFingerprint != item.Fingerprint {
		return "", "", fmt.Errorf("staged materialization fingerprint mismatch at %s", item.Path)
	}
	if _, err := os.Lstat(item.Path); err == nil {
		if err := os.Remove(item.Path); err != nil {
			return "", "", err
		}
	}
	if err := os.Rename(tempPath, item.Path); err != nil {
		return "", "", err
	}
	if err := syncDirectory(filepath.Dir(item.Path)); err != nil {
		return "", "", err
	}
	postFingerprint, err := fingerprintTree(item.Path)
	if err != nil || postFingerprint != item.Fingerprint {
		return "", "", fmt.Errorf("materialized target verification failed at %s", item.Path)
	}
	return "materialized", postFingerprint, nil
}

func bindingFingerprint(path string, info os.FileInfo) (string, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		_, fingerprint, err := fingerprintResolved(path)
		return fingerprint, err
	}
	return fingerprintTree(path)
}

func backupRewrittenPaths(items []JournalItem, destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	for index, journalItem := range items {
		if _, err := os.Lstat(journalItem.Item.Path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		backupPath := filepath.Join(destination, fmt.Sprintf("%04d", index))
		if err := copyPath(journalItem.Item.Path, backupPath); err != nil {
			return err
		}
	}
	return syncDirectory(destination)
}

func backupLegacyMetadata(legacyDir, destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	for _, relative := range []string{
		"manifest.json",
		"agents-manifest.json",
		"history",
		"pending-conflicts",
		"slash-commands",
		"mcp",
	} {
		source := filepath.Join(legacyDir, relative)
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyPath(source, filepath.Join(destination, relative)); err != nil {
			return err
		}
	}
	return syncDirectory(destination)
}

func writeBackupChecksums(backupPath string) error {
	checksums := make(map[string]string)
	for _, relative := range []string{
		"legacy-data",
		"legacy-metadata",
		"rewritten-paths",
		"rewritten-paths.json",
		"migration-loss-report.json",
	} {
		fingerprint, err := fingerprintTree(filepath.Join(backupPath, relative))
		if err != nil {
			return err
		}
		checksums[relative] = fingerprint
	}
	return atomicWriteJSON(filepath.Join(backupPath, "backup-checksums.json"), checksums, 0o600)
}

func verifyBackupChecksums(backupPath string) error {
	var checksums map[string]string
	if err := readJSON(filepath.Join(backupPath, "backup-checksums.json"), &checksums); err != nil {
		return err
	}
	if len(checksums) == 0 {
		return fmt.Errorf("backup checksum manifest is empty")
	}
	for relative, expected := range checksums {
		path := filepath.Join(backupPath, relative)
		if !pathWithin(path, backupPath) {
			return fmt.Errorf("unsafe backup checksum path")
		}
		actual, err := fingerprintTree(path)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("backup checksum mismatch: %s", relative)
		}
	}
	return nil
}

func materializationTempPath(target, operationID string) string {
	suffix := operationID
	if suffix == "" {
		suffix = "in-progress"
	}
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".harnesskit-migrate-"+suffix)
}

func CleanStart(ctx context.Context, cfg Config) (Result, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfigPaths(cfg); err != nil {
		return Result{}, err
	}
	lock, err := acquireLock(cfg)
	if err != nil {
		return Result{}, err
	}
	defer lock.release()
	if err := ensureServicesStopped(cfg, false); err != nil {
		return Result{}, err
	}
	markerPath := filepath.Join(cfg.HarnessKitDataDir, "migrations", markerFileName)
	var marker Marker
	if err := readJSON(markerPath, &marker); err == nil && marker.RolledBackAt == nil {
		return Result{Status: "already-completed", Mode: marker.Mode, BackupID: marker.BackupID, MarkerPath: markerPath}, nil
	}
	if err := cfg.HKRunner(ctx, cfg); err != nil {
		return Result{}, err
	}
	now := cfg.Now().UTC()
	if err := atomicWriteJSON(markerPath, Marker{
		Version: planVersion, MigrationID: migrationID, Mode: "clean-start", CompletedAt: now,
	}, 0o600); err != nil {
		return Result{}, err
	}
	return Result{Status: "completed", Mode: "clean-start", MarkerPath: markerPath}, nil
}

func operationIdentifier(now time.Time) string {
	random := make([]byte, 4)
	_, _ = rand.Read(random)
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(random)
}

func publicOperationError(err error) string {
	message := err.Error()
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}

func validateBackupID(id string) error {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\`) || !strings.HasPrefix(id, "skills-manager-") {
		return fmt.Errorf("invalid backup ID")
	}
	return nil
}

func validateJournal(cfg Config, journal Journal) error {
	if err := validateBackupID(journal.BackupID); err != nil {
		return err
	}
	if journal.OperationID == "" || filepath.Base(journal.OperationID) != journal.OperationID ||
		strings.ContainsAny(journal.OperationID, `/\`) {
		return fmt.Errorf("invalid operation ID")
	}
	expectedBackupPath := filepath.Join(cfg.BackupRoot, journal.BackupID)
	if filepath.Clean(journal.BackupPath) != filepath.Clean(expectedBackupPath) {
		return fmt.Errorf("journal backup path is outside the configured backup root")
	}
	resolvedLegacy := cfg.LegacyDir
	if resolved, err := filepath.EvalSymlinks(cfg.LegacyDir); err == nil {
		resolvedLegacy = resolved
	}
	resolvedLegacy, _ = filepath.Abs(resolvedLegacy)
	roots := knownBindingRoots(cfg)
	for _, journalItem := range journal.Items {
		item := journalItem.Item
		if item.Action != "materialize" && item.Action != "copy-orphan" {
			return fmt.Errorf("unsupported journal action %q", item.Action)
		}
		if item.Path == "" || item.SourcePath == "" || item.Fingerprint == "" ||
			!pathWithin(item.SourcePath, resolvedLegacy) {
			return fmt.Errorf("journal item is outside the legacy source")
		}
		targetAllowed := false
		for _, root := range roots {
			if filepath.Clean(filepath.Dir(item.Path)) == filepath.Clean(root.path) {
				targetAllowed = true
				break
			}
		}
		if item.Action == "materialize" {
			for _, configPath := range knownMCPConfigPaths(cfg) {
				if filepath.Clean(item.Path) == filepath.Clean(configPath) {
					targetAllowed = true
					break
				}
			}
		}
		if !targetAllowed {
			return fmt.Errorf("journal target is outside known Agent-native roots: %s", item.Path)
		}
		if item.Action == "materialize" && item.LinkTarget == "" {
			return fmt.Errorf("materialization journal item has no original symlink target")
		}
		if item.Action == "copy-orphan" {
			if item.Kind != "skill" && item.Kind != "subagent" {
				return fmt.Errorf("unsupported orphan import kind %q", item.Kind)
			}
			allowedCopyTarget := filepath.Join(cfg.Home, ".agents", "skills")
			if item.Kind == "subagent" {
				allowedCopyTarget = filepath.Join(cfg.Home, ".claude", "agents")
			}
			if filepath.Clean(filepath.Dir(item.Path)) != filepath.Clean(allowedCopyTarget) {
				return fmt.Errorf("orphan import target is outside the preferred native root")
			}
		}
	}
	return nil
}
