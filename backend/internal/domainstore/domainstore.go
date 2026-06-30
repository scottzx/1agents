// Package domainstore provides helpers for the two storage faces that apps own:
//
//  1. meta.db domain tables — idempotent CREATE TABLE IF NOT EXISTS, namespaced
//     by appID (e.g. "media_content_project", "media_material"). Tables store
//     structured metadata and file paths; never raw binary blobs.
//
//  2. File artifact storage — each app/project gets a directory under the
//     workspace path. Binary artifacts (MP3, images, videos, etc.) live on the
//     file face; meta.db rows carry the relative or absolute path.
//
// Design constraints (§9 三存储面):
//   - Domain tables MUST be prefixed by appID + "_".
//   - Binary artifacts go on disk; domain table rows reference the path.
//   - This package MUST NOT import app code. Apps import it.
package domainstore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureTables runs each DDL statement in ddls against db. Every statement
// must use CREATE TABLE IF NOT EXISTS and the table name must carry the
// appID + "_" prefix. Idempotent — safe to call every startup. Does NOT
// touch the global schemaVersion counter.
//
// Wave 3 apps call this from their init() or app-startup hook via
// appregistry.EnsureDomainTables, which delegates here.
func EnsureTables(db *sql.DB, appID string, ddls []string) error {
	prefix := strings.ToLower(appID + "_")
	for _, ddl := range ddls {
		if !strings.Contains(strings.ToLower(ddl), prefix) {
			return fmt.Errorf("domainstore: DDL for app %q must reference a table with prefix %q\nDDL: %s",
				appID, prefix, ddl)
		}
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("domainstore: exec DDL for app %s: %w", appID, err)
		}
	}
	return nil
}

// ArtifactDir resolves (and creates) the artifact directory for an app
// within a workspace. sub is an optional list of subdirectory components.
//
// Layout: <workspacePath>/.artifacts/<appID>[/<sub[0]>/<sub[1]>/...]
//
// Returns the absolute path of the created directory. The caller stores this
// path (or a relative variant) in a domain table row; never the file bytes.
//
// Example:
//
//	dir, err := domainstore.ArtifactDir("/home/user/projects/podcast", "radio", "episodes")
//	// → /home/user/projects/podcast/.artifacts/radio/episodes
func ArtifactDir(workspacePath, appID string, sub ...string) (string, error) {
	if workspacePath == "" {
		return "", fmt.Errorf("domainstore: workspacePath must not be empty")
	}
	if appID == "" {
		return "", fmt.Errorf("domainstore: appID must not be empty")
	}
	parts := append([]string{workspacePath, ".artifacts", appID}, sub...)
	dir := filepath.Join(parts...)
	dir = filepath.Clean(dir)
	// Prevent directory traversal outside the workspace.
	root := filepath.Clean(workspacePath)
	if !strings.HasPrefix(dir, root) {
		return "", fmt.Errorf("domainstore: path traversal detected: %s escapes %s", dir, root)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("domainstore: mkdir %s: %w", dir, err)
	}
	return dir, nil
}

// RelativePath returns dir as a path relative to workspacePath. Useful for
// storing compact paths in domain table rows.
func RelativePath(workspacePath, dir string) (string, error) {
	rel, err := filepath.Rel(workspacePath, dir)
	if err != nil {
		return "", fmt.Errorf("domainstore: rel %s → %s: %w", workspacePath, dir, err)
	}
	return rel, nil
}
