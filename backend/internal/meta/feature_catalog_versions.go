package meta

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

var (
	ErrFeatureCatalogInvalidSnapshot = errors.New("meta: invalid feature catalog snapshot")
	ErrFeatureCatalogInvalidVersion  = errors.New("meta: invalid feature catalog version")
)

const (
	featureCatalogSnapshotSchemaVersion = 1
	featureCatalogVersionPageSize       = 50
	featureCatalogRestoreWarningLimit   = 100
)

type featureCatalogSnapshotNode struct {
	ID                string          `json:"id"`
	ParentID          string          `json:"parentId"`
	Kind              FeatureNodeKind `json:"kind"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	Documents         []string        `json:"documents,omitempty"`
	TargetMilestoneID string          `json:"targetMilestoneId"`
	Position          int             `json:"position"`
	CreatedAt         string          `json:"createdAt"`
	UpdatedAt         string          `json:"updatedAt"`
}

type featureCatalogSnapshotLink struct {
	FeatureID string              `json:"featureId"`
	ItemID    string              `json:"itemId"`
	Relation  FeatureItemRelation `json:"relation"`
	CreatedAt string              `json:"createdAt"`
}

type featureCatalogSnapshot struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Nodes         []featureCatalogSnapshotNode `json:"nodes"`
	Links         []featureCatalogSnapshotLink `json:"links"`
}

type featureCatalogVersionRecord struct {
	FeatureCatalogVersion
	SnapshotJSON string
}

func scanFeatureCatalogVersion(row rowScanner, withSnapshot bool) (featureCatalogVersionRecord, error) {
	var record featureCatalogVersionRecord
	var createdAt, updatedAt string
	var err error
	if withSnapshot {
		err = row.Scan(
			&record.ID, &record.ProjectID, &record.Alias, &record.Kind,
			&record.SchemaVersion, &record.SnapshotJSON, &record.NodeCount,
			&record.LinkCount, &createdAt, &updatedAt,
		)
	} else {
		err = row.Scan(
			&record.ID, &record.ProjectID, &record.Alias, &record.Kind,
			&record.SchemaVersion, &record.NodeCount, &record.LinkCount,
			&createdAt, &updatedAt,
		)
	}
	if err != nil {
		return featureCatalogVersionRecord{}, err
	}
	record.CreatedAt = strToTime(createdAt)
	record.UpdatedAt = strToTime(updatedAt)
	return record, nil
}

const featureCatalogVersionMetadataCols = `id, project_id, alias, kind,
	schema_version, node_count, link_count, created_at, updated_at`

const featureCatalogVersionSnapshotCols = `id, project_id, alias, kind,
	schema_version, snapshot_json, node_count, link_count, created_at, updated_at`

func featureCatalogVersionTx(tx *sql.Tx, projectID, versionID string, withSnapshot bool) (featureCatalogVersionRecord, error) {
	columns := featureCatalogVersionMetadataCols
	if withSnapshot {
		columns = featureCatalogVersionSnapshotCols
	}
	record, err := scanFeatureCatalogVersion(tx.QueryRow(
		`SELECT `+columns+` FROM feature_catalog_versions WHERE project_id = ? AND id = ?`,
		projectID, versionID,
	), withSnapshot)
	if err == sql.ErrNoRows {
		return featureCatalogVersionRecord{}, ErrNotFound
	}
	return record, err
}

func parseSnapshotTime(value string) (time.Time, error) {
	if value == "" || !strings.HasSuffix(value, "Z") {
		return time.Time{}, ErrFeatureCatalogInvalidSnapshot
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.IsZero() {
		return time.Time{}, ErrFeatureCatalogInvalidSnapshot
	}
	return parsed.UTC(), nil
}

func decodeFeatureCatalogSnapshot(raw []byte) (featureCatalogSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot featureCatalogSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return featureCatalogSnapshot{}, fmt.Errorf("%w: %v", ErrFeatureCatalogInvalidSnapshot, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return featureCatalogSnapshot{}, ErrFeatureCatalogInvalidSnapshot
	}
	if err := validateFeatureCatalogSnapshot(snapshot); err != nil {
		return featureCatalogSnapshot{}, err
	}
	return snapshot, nil
}

func validateFeatureCatalogSnapshot(snapshot featureCatalogSnapshot) error {
	invalid := func(detail string) error {
		return fmt.Errorf("%w: %s", ErrFeatureCatalogInvalidSnapshot, detail)
	}
	if snapshot.SchemaVersion != featureCatalogSnapshotSchemaVersion {
		return invalid("unsupported schema version")
	}
	if snapshot.Nodes == nil || snapshot.Links == nil {
		return invalid("nodes and links are required arrays")
	}
	byID := make(map[string]featureCatalogSnapshotNode, len(snapshot.Nodes))
	children := make(map[string][]featureCatalogSnapshotNode)
	for _, node := range snapshot.Nodes {
		if node.ID == "" || strings.TrimSpace(node.Title) == "" || node.Position < 0 {
			return invalid("invalid node")
		}
		if _, exists := byID[node.ID]; exists {
			return invalid("duplicate node id")
		}
		if node.Kind != FeatureNodeModule && node.Kind != FeatureNodePoint {
			return invalid("invalid node kind")
		}
		if _, err := parseSnapshotTime(node.CreatedAt); err != nil {
			return invalid("invalid node createdAt")
		}
		if _, err := parseSnapshotTime(node.UpdatedAt); err != nil {
			return invalid("invalid node updatedAt")
		}
		if node.Kind == FeatureNodePoint && node.ParentID == "" {
			return invalid("feature at root")
		}
		byID[node.ID] = node
		children[node.ParentID] = append(children[node.ParentID], node)
	}
	for _, node := range snapshot.Nodes {
		if node.ParentID == "" {
			continue
		}
		parent, ok := byID[node.ParentID]
		if !ok {
			return invalid("missing parent")
		}
		if parent.Kind != FeatureNodeModule {
			return invalid("feature has children")
		}
	}
	for _, siblings := range children {
		positions := make([]int, len(siblings))
		for i, node := range siblings {
			positions[i] = node.Position
		}
		sort.Ints(positions)
		for i, position := range positions {
			if position != i {
				return invalid("non-contiguous sibling positions")
			}
		}
	}
	visited := make(map[string]bool, len(snapshot.Nodes))
	visiting := make(map[string]bool, len(snapshot.Nodes))
	var visit func(string, int) error
	visit = func(id string, moduleDepth int) error {
		if visiting[id] {
			return invalid("cycle")
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		node := byID[id]
		if node.Kind == FeatureNodeModule {
			moduleDepth++
			if moduleDepth > MaxFeatureModuleDepth {
				return invalid("module depth exceeds nine")
			}
		}
		for _, child := range children[id] {
			if err := visit(child.ID, moduleDepth); err != nil {
				return err
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for _, root := range children[""] {
		if err := visit(root.ID, 0); err != nil {
			return err
		}
	}
	if len(visited) != len(snapshot.Nodes) {
		return invalid("unreachable cycle")
	}

	links := make(map[string]bool, len(snapshot.Links))
	for _, link := range snapshot.Links {
		if link.FeatureID == "" || link.ItemID == "" {
			return invalid("invalid link")
		}
		node, ok := byID[link.FeatureID]
		if !ok || node.Kind != FeatureNodePoint {
			return invalid("link does not reference a feature")
		}
		if link.Relation != FeatureItemSource && link.Relation != FeatureItemDelivery {
			return invalid("invalid link relation")
		}
		if _, err := parseSnapshotTime(link.CreatedAt); err != nil {
			return invalid("invalid link createdAt")
		}
		key := link.FeatureID + "\x00" + string(link.Relation) + "\x00" + link.ItemID
		if links[key] {
			return invalid("duplicate link")
		}
		links[key] = true
	}
	return nil
}

func buildFeatureCatalogSnapshotTx(tx *sql.Tx, projectID string) (featureCatalogSnapshot, []byte, error) {
	snapshot := featureCatalogSnapshot{
		SchemaVersion: featureCatalogSnapshotSchemaVersion,
		Nodes:         []featureCatalogSnapshotNode{},
		Links:         []featureCatalogSnapshotLink{},
	}
	rows, err := tx.Query(`
		SELECT `+featureNodeCols+` FROM feature_nodes
		WHERE project_id = ? ORDER BY parent_id, position, created_at, id`, projectID)
	if err != nil {
		return snapshot, nil, err
	}
	for rows.Next() {
		node, err := scanFeatureNode(rows)
		if err != nil {
			rows.Close()
			return snapshot, nil, err
		}
		snapshot.Nodes = append(snapshot.Nodes, featureCatalogSnapshotNode{
			ID: node.ID, ParentID: node.ParentID, Kind: node.Kind,
			Title: node.Title, Description: node.Description, Documents: node.Documents,
			TargetMilestoneID: node.TargetMilestoneID, Position: node.Position,
			CreatedAt: timeToStr(node.CreatedAt), UpdatedAt: timeToStr(node.UpdatedAt),
		})
	}
	if err := rows.Close(); err != nil {
		return snapshot, nil, err
	}
	if err := rows.Err(); err != nil {
		return snapshot, nil, err
	}

	linkRows, err := tx.Query(`
		SELECT l.feature_id, l.item_id, l.relation, l.created_at
		FROM feature_item_links l
		JOIN feature_nodes n ON n.id = l.feature_id
		WHERE n.project_id = ?
		ORDER BY l.feature_id, l.relation, l.item_id, l.created_at`, projectID)
	if err != nil {
		return snapshot, nil, err
	}
	for linkRows.Next() {
		var link featureCatalogSnapshotLink
		if err := linkRows.Scan(&link.FeatureID, &link.ItemID, &link.Relation, &link.CreatedAt); err != nil {
			linkRows.Close()
			return snapshot, nil, err
		}
		snapshot.Links = append(snapshot.Links, link)
	}
	if err := linkRows.Close(); err != nil {
		return snapshot, nil, err
	}
	if err := linkRows.Err(); err != nil {
		return snapshot, nil, err
	}
	raw, err := json.Marshal(snapshot)
	return snapshot, raw, err
}

func insertFeatureCatalogVersionTx(
	tx *sql.Tx,
	projectID, alias string,
	kind FeatureCatalogVersionKind,
	snapshot featureCatalogSnapshot,
	raw []byte,
	at time.Time,
) (FeatureCatalogVersion, error) {
	if kind != FeatureCatalogVersionManual && kind != FeatureCatalogVersionPreRestore {
		return FeatureCatalogVersion{}, ErrFeatureCatalogInvalidVersion
	}
	version := FeatureCatalogVersion{
		ID: newID(), ProjectID: projectID, Alias: strings.TrimSpace(alias),
		Kind: kind, SchemaVersion: featureCatalogSnapshotSchemaVersion,
		NodeCount: len(snapshot.Nodes), LinkCount: len(snapshot.Links),
		CreatedAt: at.UTC(), UpdatedAt: at.UTC(),
	}
	_, err := tx.Exec(`
		INSERT INTO feature_catalog_versions (
			id, project_id, alias, kind, schema_version, snapshot_json,
			node_count, link_count, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version.ID, projectID, version.Alias, version.Kind, version.SchemaVersion,
		string(raw), version.NodeCount, version.LinkCount,
		timeToStr(version.CreatedAt), timeToStr(version.UpdatedAt),
	)
	return version, err
}

func featureCatalogVersionSnapshot(version FeatureCatalogVersion) map[string]any {
	return map[string]any{
		"id": version.ID, "alias": version.Alias, "kind": version.Kind,
		"schemaVersion": version.SchemaVersion, "nodeCount": version.NodeCount,
		"linkCount": version.LinkCount, "createdAt": version.CreatedAt,
	}
}

func (s *FeatureCatalogStore) ListVersions(projectID, cursorValue string) (FeatureCatalogVersionPage, error) {
	cursor, err := decodeStoreCursor(cursorValue)
	if err != nil {
		return FeatureCatalogVersionPage{}, err
	}
	query := `SELECT ` + featureCatalogVersionMetadataCols + `
		FROM feature_catalog_versions WHERE project_id = ?`
	args := []any{projectID}
	if cursor.At != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, cursor.At, cursor.At, cursor.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, featureCatalogVersionPageSize+1)
	rows, err := s.db.sql.Query(query, args...)
	if err != nil {
		return FeatureCatalogVersionPage{}, err
	}
	defer rows.Close()
	items := make([]FeatureCatalogVersion, 0, featureCatalogVersionPageSize+1)
	for rows.Next() {
		record, err := scanFeatureCatalogVersion(rows, false)
		if err != nil {
			return FeatureCatalogVersionPage{}, err
		}
		items = append(items, record.FeatureCatalogVersion)
	}
	if err := rows.Err(); err != nil {
		return FeatureCatalogVersionPage{}, err
	}
	page := FeatureCatalogVersionPage{Items: items}
	if len(items) > featureCatalogVersionPageSize {
		page.Items = items[:featureCatalogVersionPageSize]
		page.HasMore = true
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeStoreCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *FeatureCatalogStore) CreateVersion(projectID, alias string, event ProjectEvent) (FeatureCatalogVersion, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return FeatureCatalogVersion{}, err
	}
	defer tx.Rollback()
	if err := ensureFeatureProjectTx(tx, projectID); err != nil {
		return FeatureCatalogVersion{}, err
	}
	snapshot, raw, err := buildFeatureCatalogSnapshotTx(tx, projectID)
	if err != nil {
		return FeatureCatalogVersion{}, err
	}
	if err := validateFeatureCatalogSnapshot(snapshot); err != nil {
		return FeatureCatalogVersion{}, err
	}
	version, err := insertFeatureCatalogVersionTx(
		tx, projectID, alias, FeatureCatalogVersionManual, snapshot, raw, time.Now().UTC(),
	)
	if err != nil {
		return FeatureCatalogVersion{}, err
	}
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_catalog_version", version.ID, "create",
		map[string]any{}, featureCatalogVersionSnapshot(version),
	); err != nil {
		return FeatureCatalogVersion{}, err
	}
	return version, tx.Commit()
}

func (s *FeatureCatalogStore) RenameVersion(
	projectID, versionID, alias string,
	event ProjectEvent,
) (FeatureCatalogVersion, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return FeatureCatalogVersion{}, err
	}
	defer tx.Rollback()
	current, err := featureCatalogVersionTx(tx, projectID, versionID, false)
	if err != nil {
		return FeatureCatalogVersion{}, err
	}
	alias = strings.TrimSpace(alias)
	now := time.Now().UTC()
	if _, err := tx.Exec(`
		UPDATE feature_catalog_versions SET alias = ?, updated_at = ?
		WHERE project_id = ? AND id = ?`,
		alias, timeToStr(now), projectID, versionID,
	); err != nil {
		return FeatureCatalogVersion{}, err
	}
	next := current.FeatureCatalogVersion
	next.Alias = alias
	next.UpdatedAt = now
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_catalog_version", versionID, "rename",
		featureCatalogVersionSnapshot(current.FeatureCatalogVersion),
		featureCatalogVersionSnapshot(next),
	); err != nil {
		return FeatureCatalogVersion{}, err
	}
	return next, tx.Commit()
}

func (s *FeatureCatalogStore) DeleteVersion(projectID, versionID string, event ProjectEvent) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := featureCatalogVersionTx(tx, projectID, versionID, false)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM feature_catalog_versions WHERE project_id = ? AND id = ?`,
		projectID, versionID,
	); err != nil {
		return err
	}
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_catalog_version", versionID, "delete",
		featureCatalogVersionSnapshot(current.FeatureCatalogVersion), map[string]any{},
	); err != nil {
		return err
	}
	return tx.Commit()
}

func restoreWarning(
	result *FeatureCatalogRestoreResult,
	warning FeatureCatalogRestoreWarning,
) {
	if len(result.Warnings) < featureCatalogRestoreWarningLimit {
		result.Warnings = append(result.Warnings, warning)
	} else {
		result.WarningsTruncated = true
	}
}

func (s *FeatureCatalogStore) RestoreVersion(
	projectID, versionID, requestID string,
	event ProjectEvent,
) (FeatureCatalogRestoreResult, error) {
	if strings.TrimSpace(requestID) == "" {
		return FeatureCatalogRestoreResult{}, ErrFeatureCatalogInvalidVersion
	}
	var existingResult string
	err := s.db.sql.QueryRow(`
		SELECT result_json FROM feature_catalog_restore_requests
		WHERE project_id = ? AND request_id = ?`,
		projectID, requestID,
	).Scan(&existingResult)
	if err == nil {
		var result FeatureCatalogRestoreResult
		if json.Unmarshal([]byte(existingResult), &result) != nil {
			return FeatureCatalogRestoreResult{}, ErrFeatureCatalogInvalidVersion
		}
		return result, nil
	}
	if err != sql.ErrNoRows {
		return FeatureCatalogRestoreResult{}, err
	}
	preflight, err := scanFeatureCatalogVersion(s.db.sql.QueryRow(`
		SELECT `+featureCatalogVersionSnapshotCols+`
		FROM feature_catalog_versions WHERE project_id = ? AND id = ?`,
		projectID, versionID,
	), true)
	if err == sql.ErrNoRows {
		return FeatureCatalogRestoreResult{}, ErrNotFound
	}
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	snapshot, err := decodeFeatureCatalogSnapshot([]byte(preflight.SnapshotJSON))
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	if preflight.SchemaVersion != featureCatalogSnapshotSchemaVersion ||
		preflight.NodeCount != len(snapshot.Nodes) ||
		preflight.LinkCount != len(snapshot.Links) {
		return FeatureCatalogRestoreResult{}, ErrFeatureCatalogInvalidSnapshot
	}

	tx, err := s.db.sql.Begin()
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	defer tx.Rollback()
	var storedResult string
	err = tx.QueryRow(`
		SELECT result_json FROM feature_catalog_restore_requests
		WHERE project_id = ? AND request_id = ?`,
		projectID, requestID,
	).Scan(&storedResult)
	if err == nil {
		var result FeatureCatalogRestoreResult
		if json.Unmarshal([]byte(storedResult), &result) != nil {
			return FeatureCatalogRestoreResult{}, ErrFeatureCatalogInvalidVersion
		}
		return result, nil
	}
	if err != sql.ErrNoRows {
		return FeatureCatalogRestoreResult{}, err
	}
	target, err := featureCatalogVersionTx(tx, projectID, versionID, true)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	if target.SnapshotJSON != preflight.SnapshotJSON ||
		target.SchemaVersion != preflight.SchemaVersion ||
		target.NodeCount != preflight.NodeCount ||
		target.LinkCount != preflight.LinkCount {
		return FeatureCatalogRestoreResult{}, ErrFeatureCatalogInvalidVersion
	}
	for _, node := range snapshot.Nodes {
		var owner string
		err := tx.QueryRow(`SELECT project_id FROM feature_nodes WHERE id = ?`, node.ID).Scan(&owner)
		if err == nil && owner != projectID {
			return FeatureCatalogRestoreResult{}, ErrFeatureCatalogInvalidSnapshot
		}
		if err != nil && err != sql.ErrNoRows {
			return FeatureCatalogRestoreResult{}, err
		}
	}

	safetySnapshot, safetyRaw, err := buildFeatureCatalogSnapshotTx(tx, projectID)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	restoreAt := time.Now().UTC()
	safety, err := insertFeatureCatalogVersionTx(
		tx, projectID, "", FeatureCatalogVersionPreRestore,
		safetySnapshot, safetyRaw, restoreAt,
	)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	safetyEvent := event
	safetyEvent.ID = newID()
	if err := appendFeatureEventTx(
		tx, safetyEvent, projectID, "feature_catalog_version", safety.ID, "create",
		map[string]any{}, featureCatalogVersionSnapshot(safety),
	); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}

	itemTypes := map[string]ItemType{}
	itemRows, err := tx.Query(`SELECT id, type FROM project_items WHERE project_id = ?`, projectID)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	for itemRows.Next() {
		var id string
		var kind ItemType
		if err := itemRows.Scan(&id, &kind); err != nil {
			itemRows.Close()
			return FeatureCatalogRestoreResult{}, err
		}
		itemTypes[id] = kind
	}
	if err := itemRows.Close(); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	if err := itemRows.Err(); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	validMilestones := map[string]bool{}
	milestoneRows, err := tx.Query(`
		SELECT id, version FROM milestones WHERE project_id = ? AND version != ''`, projectID)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	for milestoneRows.Next() {
		var id, version string
		if err := milestoneRows.Scan(&id, &version); err != nil {
			milestoneRows.Close()
			return FeatureCatalogRestoreResult{}, err
		}
		if _, valid := parseMilestoneSemVer(version); valid {
			validMilestones[id] = true
		}
	}
	if err := milestoneRows.Close(); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	if err := milestoneRows.Err(); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}

	result := FeatureCatalogRestoreResult{
		RequestID: requestID, TargetVersion: target.FeatureCatalogVersion,
		SafetyVersion: safety, RestoredNodeCount: len(snapshot.Nodes),
		Warnings: []FeatureCatalogRestoreWarning{},
	}
	validLinks := make([]featureCatalogSnapshotLink, 0, len(snapshot.Links))
	for _, link := range snapshot.Links {
		itemType, exists := itemTypes[link.ItemID]
		valid := exists
		if link.Relation == FeatureItemSource {
			valid = valid && (itemType == ItemTypeRequirement || itemType == ItemTypeBug)
		} else {
			valid = valid && (itemType == "" || itemType == ItemTypeTask)
		}
		if valid {
			validLinks = append(validLinks, link)
			continue
		}
		result.SkippedLinkCount++
		kind := "source_link"
		if link.Relation == FeatureItemDelivery {
			kind = "delivery_link"
		}
		restoreWarning(&result, FeatureCatalogRestoreWarning{
			FeatureID: link.FeatureID, ReferenceID: link.ItemID,
			Kind: kind, Action: "skipped",
		})
	}
	result.RestoredLinkCount = len(validLinks)

	if _, err := tx.Exec(`
		DELETE FROM feature_item_links WHERE feature_id IN (
			SELECT id FROM feature_nodes WHERE project_id = ?
		)`, projectID); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	if _, err := tx.Exec(`DELETE FROM feature_nodes WHERE project_id = ?`, projectID); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	nodeStatement, err := tx.Prepare(`
		INSERT INTO feature_nodes (
			id, project_id, parent_id, kind, title, description,
			documents_json, target_milestone_id, position, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	defer nodeStatement.Close()
	for _, node := range snapshot.Nodes {
		documentsJSON, err := featureDocumentsJSON(node.Documents)
		if err != nil {
			return FeatureCatalogRestoreResult{}, err
		}
		targetMilestoneID := node.TargetMilestoneID
		updatedAt := node.UpdatedAt
		if targetMilestoneID != "" && !validMilestones[targetMilestoneID] {
			result.ClearedTargetMilestoneCount++
			restoreWarning(&result, FeatureCatalogRestoreWarning{
				FeatureID: node.ID, ReferenceID: targetMilestoneID,
				Kind: "target_milestone", Action: "cleared",
			})
			targetMilestoneID = ""
			updatedAt = timeToStr(restoreAt)
		}
		if _, err := nodeStatement.Exec(
			node.ID, projectID, node.ParentID, node.Kind, node.Title,
			node.Description, documentsJSON, targetMilestoneID, node.Position,
			node.CreatedAt, updatedAt,
		); err != nil {
			return FeatureCatalogRestoreResult{}, err
		}
	}
	linkStatement, err := tx.Prepare(`
		INSERT INTO feature_item_links (feature_id, item_id, relation, created_at)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	defer linkStatement.Close()
	for _, link := range validLinks {
		if _, err := linkStatement.Exec(
			link.FeatureID, link.ItemID, link.Relation, link.CreatedAt,
		); err != nil {
			return FeatureCatalogRestoreResult{}, err
		}
	}

	restoreEvent := event
	if err := appendFeatureEventTx(
		tx, restoreEvent, projectID, "feature_catalog_version", target.ID, "restore",
		featureCatalogVersionSnapshot(safety),
		map[string]any{
			"targetVersion":               featureCatalogVersionSnapshot(target.FeatureCatalogVersion),
			"restoredNodeCount":           result.RestoredNodeCount,
			"restoredLinkCount":           result.RestoredLinkCount,
			"skippedLinkCount":            result.SkippedLinkCount,
			"clearedTargetMilestoneCount": result.ClearedTargetMilestoneCount,
		},
	); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	if _, err := tx.Exec(`
		INSERT INTO feature_catalog_restore_requests (
			project_id, request_id, version_id, safety_version_id,
			result_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, requestID, versionID, safety.ID, string(resultJSON), timeToStr(restoreAt),
	); err != nil {
		return FeatureCatalogRestoreResult{}, err
	}
	return result, tx.Commit()
}
