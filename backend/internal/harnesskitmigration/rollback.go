package harnesskitmigration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func DataRollback(ctx context.Context, cfg Config, backupID string) (RollbackReport, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfigPaths(cfg); err != nil {
		return RollbackReport{}, err
	}
	if err := validateBackupID(backupID); err != nil {
		return RollbackReport{}, err
	}
	backupPath := filepath.Join(cfg.BackupRoot, backupID)
	var journal Journal
	if err := readJSON(filepath.Join(backupPath, "migration-record.json"), &journal); err != nil {
		return RollbackReport{}, fmt.Errorf("read backup migration record: %w", err)
	}
	if journal.BackupID != backupID || journal.Phase != "committed" {
		return RollbackReport{}, fmt.Errorf("backup is not a committed HarnessKit migration")
	}
	if err := validateJournal(cfg, journal); err != nil {
		return RollbackReport{}, fmt.Errorf("validate backup migration record: %w", err)
	}
	if err := verifyBackupChecksums(backupPath); err != nil {
		return RollbackReport{}, fmt.Errorf("verify migration backup: %w", err)
	}

	lock, err := acquireLock(cfg)
	if err != nil {
		return RollbackReport{}, err
	}
	defer lock.release()
	if err := ensureServicesStopped(cfg, true); err != nil {
		return RollbackReport{}, err
	}

	postStateBackup := filepath.Join(backupPath, "harnesskit-post-rollback")
	if _, err := os.Stat(postStateBackup); os.IsNotExist(err) {
		if err := snapshotHarnessKitState(cfg.HarnessKitDataDir, postStateBackup); err != nil {
			return RollbackReport{}, fmt.Errorf("backup post-migration HarnessKit state: %w", err)
		}
	}

	report := RollbackReport{BackupID: backupID, CreatedAt: cfg.Now().UTC()}
	for _, journalItem := range journal.Items {
		item := journalItem.Item
		info, err := os.Lstat(item.Path)
		if item.Action == "copy-orphan" {
			if journalItem.Status == "unchanged" {
				if err == nil {
					report.Unchanged++
					continue
				}
				if os.IsNotExist(err) {
					report.Unchanged++
					continue
				}
				return report, err
			}
			if os.IsNotExist(err) {
				report.Unchanged++
				continue
			}
			if err != nil {
				return report, err
			}
			fingerprint, hashErr := bindingFingerprint(item.Path, info)
			if hashErr != nil || fingerprint != journalItem.PostFingerprint {
				report.Conflicts = append(report.Conflicts, Conflict{
					Path: item.Path, Kind: item.Kind,
					Reason: "post-migration content was modified; left untouched",
				})
				continue
			}
			if err := os.RemoveAll(item.Path); err != nil {
				return report, err
			}
			if err := syncDirectory(filepath.Dir(item.Path)); err != nil {
				return report, err
			}
			report.Restored++
			continue
		}
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(item.Path), 0o700); err != nil {
				return report, err
			}
			if err := os.Symlink(item.LinkTarget, item.Path); err != nil {
				return report, err
			}
			report.Restored++
			continue
		}
		if err != nil {
			return report, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(item.Path)
			if readErr == nil && target == item.LinkTarget {
				report.Unchanged++
				continue
			}
			report.Conflicts = append(report.Conflicts, Conflict{
				Path: item.Path, Kind: item.Kind,
				Reason: "current symlink differs from the pre-migration target; left untouched",
			})
			continue
		}
		fingerprint, hashErr := fingerprintTree(item.Path)
		if hashErr != nil || fingerprint != journalItem.PostFingerprint {
			report.Conflicts = append(report.Conflicts, Conflict{
				Path: item.Path, Kind: item.Kind,
				Reason: "post-migration content was modified; left untouched",
			})
			continue
		}
		if err := os.RemoveAll(item.Path); err != nil {
			return report, err
		}
		if err := os.Symlink(item.LinkTarget, item.Path); err != nil {
			return report, err
		}
		if err := syncDirectory(filepath.Dir(item.Path)); err != nil {
			return report, err
		}
		report.Restored++
	}

	if err := cfg.HKRunner(ctx, cfg); err != nil {
		return report, err
	}
	reportPath := filepath.Join(backupPath, "data-rollback-report.json")
	if err := atomicWriteJSON(reportPath, report, 0o600); err != nil {
		return report, err
	}
	markerPath := filepath.Join(cfg.HarnessKitDataDir, "migrations", markerFileName)
	var marker Marker
	if err := readJSON(markerPath, &marker); err == nil && marker.BackupID == backupID {
		rolledBackAt := cfg.Now().UTC()
		marker.RolledBackAt = &rolledBackAt
		if err := atomicWriteJSON(markerPath, marker, 0o600); err != nil {
			return report, err
		}
	}
	return report, nil
}

func snapshotHarnessKitState(source, destination string) error {
	if pathWithin(destination, source) {
		return fmt.Errorf("HarnessKit rollback snapshot must be outside its data directory")
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "migrations" {
			continue
		}
		if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return syncDirectory(destination)
}
