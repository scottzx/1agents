package meta

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrFeatureInvalidKind      = errors.New("meta: invalid feature node kind")
	ErrFeatureInvalidParent    = errors.New("meta: invalid feature parent")
	ErrFeatureMaxDepth         = errors.New("meta: feature module depth exceeds nine")
	ErrFeatureCycle            = errors.New("meta: feature tree cycle")
	ErrFeatureHasChildren      = errors.New("meta: feature node has children")
	ErrFeatureInvalidRelation  = errors.New("meta: invalid feature item relation")
	ErrFeatureInvalidItemType  = errors.New("meta: invalid project item type for feature relation")
	ErrFeatureInvalidMilestone = errors.New("meta: feature target must be a semantic version milestone")
)

const MaxFeatureModuleDepth = 9

type FeatureNodePatch struct {
	ParentID          *string
	Title             *string
	Description       *string
	Documents         *[]string
	TargetMilestoneID *string
	Position          *int
}

// FeatureCatalogBatchOperation is one atomic feature-catalog mutation. A create
// may publish clientRef; later operations can address it through parentRef,
// nodeRef, or featureRef without knowing the server-generated node id.
type FeatureCatalogBatchOperation struct {
	Operation         string              `json:"op"`
	ID                string              `json:"id,omitempty"`
	NodeRef           string              `json:"nodeRef,omitempty"`
	ClientRef         string              `json:"clientRef,omitempty"`
	ParentID          *string             `json:"parentId,omitempty"`
	ParentRef         string              `json:"parentRef,omitempty"`
	FeatureID         string              `json:"featureId,omitempty"`
	FeatureRef        string              `json:"featureRef,omitempty"`
	ItemID            string              `json:"itemId,omitempty"`
	Relation          FeatureItemRelation `json:"relation,omitempty"`
	Kind              FeatureNodeKind     `json:"kind,omitempty"`
	Title             *string             `json:"title,omitempty"`
	Description       *string             `json:"description,omitempty"`
	Documents         *[]string           `json:"documents,omitempty"`
	TargetMilestoneID *string             `json:"targetMilestoneId,omitempty"`
	Position          *int                `json:"position,omitempty"`
}

type FeatureCatalogBatchResult struct {
	Operation string           `json:"op"`
	ClientRef string           `json:"clientRef,omitempty"`
	Node      *FeatureNode     `json:"node,omitempty"`
	Link      *FeatureItemLink `json:"link,omitempty"`
	Created   *bool            `json:"created,omitempty"`
	OK        bool             `json:"ok,omitempty"`
}

type FeatureCatalogStore struct {
	db *DB
}

func NewFeatureCatalogStore(db *DB) *FeatureCatalogStore {
	return &FeatureCatalogStore{db: db}
}

const featureNodeCols = `id, project_id, parent_id, kind, title, description,
	documents_json, target_milestone_id, position, created_at, updated_at`

func scanFeatureNode(row rowScanner) (FeatureNode, error) {
	var node FeatureNode
	var documentsJSON string
	var createdAt, updatedAt string
	if err := row.Scan(
		&node.ID, &node.ProjectID, &node.ParentID, &node.Kind, &node.Title,
		&node.Description, &documentsJSON, &node.TargetMilestoneID, &node.Position,
		&createdAt, &updatedAt,
	); err != nil {
		return FeatureNode{}, err
	}
	if err := json.Unmarshal([]byte(documentsJSON), &node.Documents); err != nil {
		return FeatureNode{}, fmt.Errorf("decode feature documents: %w", err)
	}
	if node.Documents == nil {
		node.Documents = []string{}
	}
	node.CreatedAt = strToTime(createdAt)
	node.UpdatedAt = strToTime(updatedAt)
	return node, nil
}

func normalizeFeatureDocuments(documents []string) []string {
	normalized := make([]string, 0, len(documents))
	seen := map[string]bool{}
	for _, document := range documents {
		document = strings.TrimSpace(document)
		if document == "" || seen[document] {
			continue
		}
		seen[document] = true
		normalized = append(normalized, document)
	}
	return normalized
}

func featureDocumentsJSON(documents []string) (string, error) {
	raw, err := json.Marshal(normalizeFeatureDocuments(documents))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *FeatureCatalogStore) Get(projectID, id string) (FeatureNode, bool, error) {
	node, err := scanFeatureNode(s.db.sql.QueryRow(
		`SELECT `+featureNodeCols+` FROM feature_nodes WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return FeatureNode{}, false, nil
	}
	if err != nil {
		return FeatureNode{}, false, err
	}
	if node.ProjectID != projectID {
		return FeatureNode{}, false, ErrProjectMismatch
	}
	return node, true, nil
}

func (s *FeatureCatalogStore) List(projectID string) (FeatureCatalog, error) {
	rows, err := s.db.sql.Query(`
		SELECT `+featureNodeCols+` FROM feature_nodes
		WHERE project_id = ? ORDER BY parent_id, position, created_at, id`, projectID)
	if err != nil {
		return FeatureCatalog{}, err
	}
	nodes := []FeatureNode{}
	for rows.Next() {
		node, err := scanFeatureNode(rows)
		if err != nil {
			rows.Close()
			return FeatureCatalog{}, err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Close(); err != nil {
		return FeatureCatalog{}, err
	}

	linkRows, err := s.db.sql.Query(`
		SELECT l.feature_id, l.item_id, l.relation, l.created_at
		FROM feature_item_links l
		JOIN feature_nodes n ON n.id = l.feature_id
		WHERE n.project_id = ?
		ORDER BY l.created_at, l.feature_id, l.item_id, l.relation`, projectID)
	if err != nil {
		return FeatureCatalog{}, err
	}
	defer linkRows.Close()
	links := []FeatureItemLink{}
	for linkRows.Next() {
		var link FeatureItemLink
		var createdAt string
		if err := linkRows.Scan(&link.FeatureID, &link.ItemID, &link.Relation, &createdAt); err != nil {
			return FeatureCatalog{}, err
		}
		link.CreatedAt = strToTime(createdAt)
		links = append(links, link)
	}
	if err := linkRows.Err(); err != nil {
		return FeatureCatalog{}, err
	}
	if err := linkRows.Close(); err != nil {
		return FeatureCatalog{}, err
	}

	itemRows, err := s.db.sql.Query(`
		SELECT id, status FROM project_items WHERE project_id = ?`, projectID)
	if err != nil {
		return FeatureCatalog{}, err
	}
	itemStatuses := map[string]TaskStatus{}
	for itemRows.Next() {
		var itemID string
		var status TaskStatus
		if err := itemRows.Scan(&itemID, &status); err != nil {
			itemRows.Close()
			return FeatureCatalog{}, err
		}
		itemStatuses[itemID] = status
	}
	if err := itemRows.Close(); err != nil {
		return FeatureCatalog{}, err
	}
	if err := itemRows.Err(); err != nil {
		return FeatureCatalog{}, err
	}

	milestoneRows, err := s.db.sql.Query(`
		SELECT id, version FROM milestones
		WHERE project_id = ? AND version != ''`, projectID)
	if err != nil {
		return FeatureCatalog{}, err
	}
	milestoneVersions := map[string]string{}
	for milestoneRows.Next() {
		var id, version string
		if err := milestoneRows.Scan(&id, &version); err != nil {
			milestoneRows.Close()
			return FeatureCatalog{}, err
		}
		milestoneVersions[id] = version
	}
	if err := milestoneRows.Close(); err != nil {
		return FeatureCatalog{}, err
	}
	if err := milestoneRows.Err(); err != nil {
		return FeatureCatalog{}, err
	}

	deriveFeatureProgress(nodes, links, itemStatuses, milestoneVersions)
	return FeatureCatalog{Nodes: nodes, Links: links}, nil
}

func featureProgressStatus(total, completed int, statuses map[TaskStatus]bool) FeatureProgressStatus {
	if total == 0 {
		return FeatureProgressUnplanned
	}
	if completed == total {
		return FeatureProgressDelivered
	}
	if completed > 0 || statuses[TaskStatusRunning] || statuses[TaskStatusFailed] {
		return FeatureProgressInProgress
	}
	return FeatureProgressPending
}

func deriveFeatureProgress(
	nodes []FeatureNode,
	links []FeatureItemLink,
	itemStatuses map[string]TaskStatus,
	milestoneVersions map[string]string,
) {
	children := map[string][]string{}
	featureIDs := map[string]bool{}
	for _, node := range nodes {
		children[node.ParentID] = append(children[node.ParentID], node.ID)
		if node.Kind == FeatureNodePoint {
			featureIDs[node.ID] = true
		}
	}

	deliveries := map[string][]string{}
	for _, link := range links {
		if link.Relation == FeatureItemDelivery && featureIDs[link.FeatureID] {
			deliveries[link.FeatureID] = append(deliveries[link.FeatureID], link.ItemID)
		}
	}

	descendantFeatures := func(rootID string) []string {
		result := []string{}
		queue := append([]string(nil), children[rootID]...)
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if featureIDs[id] {
				result = append(result, id)
				continue
			}
			queue = append(queue, children[id]...)
		}
		return result
	}

	for i := range nodes {
		ids := []string{nodes[i].ID}
		if nodes[i].Kind == FeatureNodeModule {
			ids = descendantFeatures(nodes[i].ID)
		}

		progress := &FeatureProgress{TotalFeatures: len(ids)}
		validTasks := map[string]TaskStatus{}
		statuses := map[TaskStatus]bool{}
		for _, featureID := range ids {
			itemIDs := deliveries[featureID]
			if len(itemIDs) == 0 {
				progress.UnplannedFeatures++
				continue
			}
			featureHasValidTask := false
			for _, itemID := range itemIDs {
				status, ok := itemStatuses[itemID]
				if !ok || status == TaskStatusCancelled {
					continue
				}
				featureHasValidTask = true
				validTasks[itemID] = status
			}
			if featureHasValidTask {
				progress.CoveredFeatures++
			} else {
				progress.ReplanFeatures++
			}
		}

		for _, status := range validTasks {
			statuses[status] = true
			progress.TotalTasks++
			if status == TaskStatusCompleted {
				progress.CompletedTasks++
			}
		}
		if progress.TotalTasks == 0 {
			if progress.ReplanFeatures > 0 {
				progress.Status = FeatureProgressReplan
			} else {
				progress.Status = FeatureProgressUnplanned
			}
		} else {
			progress.Status = featureProgressStatus(progress.TotalTasks, progress.CompletedTasks, statuses)
			percent := progress.CompletedTasks * 100 / progress.TotalTasks
			progress.ProgressPercent = &percent
		}
		nodes[i].Progress = progress

		if nodes[i].Kind == FeatureNodeModule {
			counts := map[string]int{}
			for _, featureID := range ids {
				for _, candidate := range nodes {
					if candidate.ID == featureID && candidate.TargetMilestoneID != "" {
						if _, ok := milestoneVersions[candidate.TargetMilestoneID]; ok {
							counts[candidate.TargetMilestoneID]++
						}
						break
					}
				}
			}
			coverage := make([]FeatureVersionCoverage, 0, len(counts))
			for milestoneID, count := range counts {
				coverage = append(coverage, FeatureVersionCoverage{
					MilestoneID:  milestoneID,
					Version:      milestoneVersions[milestoneID],
					FeatureCount: count,
				})
			}
			sort.Slice(coverage, func(a, b int) bool {
				left, leftOK := parseMilestoneSemVer(coverage[a].Version)
				right, rightOK := parseMilestoneSemVer(coverage[b].Version)
				if leftOK && rightOK {
					return left.less(right)
				}
				return coverage[a].Version < coverage[b].Version
			})
			nodes[i].VersionCoverage = coverage
		}
	}
}

func validateFeatureMilestoneTx(tx *sql.Tx, projectID, milestoneID string) (Milestone, error) {
	if milestoneID == "" {
		return Milestone{}, nil
	}
	var milestone Milestone
	if err := tx.QueryRow(`
		SELECT project_id, name, version FROM milestones WHERE id = ?`,
		milestoneID,
	).Scan(&milestone.ProjectID, &milestone.Name, &milestone.Version); err != nil {
		if err == sql.ErrNoRows {
			return Milestone{}, ErrNotFound
		}
		return Milestone{}, err
	}
	if milestone.ProjectID != projectID {
		return Milestone{}, ErrProjectMismatch
	}
	if milestone.Version == "" {
		return Milestone{}, ErrFeatureInvalidMilestone
	}
	milestone.ID = milestoneID
	return milestone, nil
}

func featureNodeTx(tx *sql.Tx, id string) (FeatureNode, error) {
	node, err := scanFeatureNode(tx.QueryRow(
		`SELECT `+featureNodeCols+` FROM feature_nodes WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return FeatureNode{}, ErrNotFound
	}
	return node, err
}

func ensureFeatureProjectTx(tx *sql.Tx, projectID string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM projects WHERE id = ?`, projectID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func moduleDepthTx(tx *sql.Tx, projectID, id string) (int, error) {
	depth := 0
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return 0, ErrFeatureCycle
		}
		seen[id] = true
		node, err := featureNodeTx(tx, id)
		if err != nil {
			return 0, err
		}
		if node.ProjectID != projectID {
			return 0, ErrProjectMismatch
		}
		if node.Kind != FeatureNodeModule {
			return 0, ErrFeatureInvalidParent
		}
		depth++
		id = node.ParentID
	}
	return depth, nil
}

func moduleSubtreeHeightTx(tx *sql.Tx, projectID, id string) (int, error) {
	var height int
	err := tx.QueryRow(`
		WITH RECURSIVE modules(id, depth) AS (
			SELECT id, 1 FROM feature_nodes WHERE id = ? AND project_id = ?
			UNION ALL
			SELECT n.id, modules.depth + 1
			FROM feature_nodes n JOIN modules ON n.parent_id = modules.id
			WHERE n.project_id = ? AND n.kind = 'module'
		)
		SELECT COALESCE(MAX(depth), 1) FROM modules`, id, projectID, projectID).Scan(&height)
	return height, err
}

func validateFeaturePlacementTx(tx *sql.Tx, node FeatureNode, parentID string) error {
	switch node.Kind {
	case FeatureNodeModule:
	case FeatureNodePoint:
		if parentID == "" {
			return ErrFeatureInvalidParent
		}
	default:
		return ErrFeatureInvalidKind
	}
	if parentID == "" {
		return nil
	}
	if parentID == node.ID {
		return ErrFeatureCycle
	}
	parent, err := featureNodeTx(tx, parentID)
	if err != nil {
		return err
	}
	if parent.ProjectID != node.ProjectID {
		return ErrProjectMismatch
	}
	if parent.Kind != FeatureNodeModule {
		return ErrFeatureInvalidParent
	}
	for ancestorID := parentID; ancestorID != ""; {
		if ancestorID == node.ID {
			return ErrFeatureCycle
		}
		ancestor, err := featureNodeTx(tx, ancestorID)
		if err != nil {
			return err
		}
		ancestorID = ancestor.ParentID
	}
	parentDepth, err := moduleDepthTx(tx, node.ProjectID, parentID)
	if err != nil {
		return err
	}
	if node.Kind == FeatureNodeModule {
		height := 1
		if node.ID != "" {
			height, err = moduleSubtreeHeightTx(tx, node.ProjectID, node.ID)
			if err != nil {
				return err
			}
		}
		if parentDepth+height > MaxFeatureModuleDepth {
			return ErrFeatureMaxDepth
		}
	}
	return nil
}

func clampPosition(position, length int) int {
	if position < 0 {
		return 0
	}
	if position > length {
		return length
	}
	return position
}

func siblingIDsTx(tx *sql.Tx, projectID, parentID, excludeID string) ([]string, error) {
	rows, err := tx.Query(`
		SELECT id FROM feature_nodes
		WHERE project_id = ? AND parent_id = ? AND id != ?
		ORDER BY position, created_at, id`, projectID, parentID, excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func placeFeatureNodeTx(tx *sql.Tx, projectID, parentID, id string, position int) error {
	ids, err := siblingIDsTx(tx, projectID, parentID, id)
	if err != nil {
		return err
	}
	position = clampPosition(position, len(ids))
	ids = append(ids, "")
	copy(ids[position+1:], ids[position:])
	ids[position] = id
	for i, siblingID := range ids {
		if _, err := tx.Exec(
			`UPDATE feature_nodes SET parent_id = ?, position = ? WHERE project_id = ? AND id = ?`,
			parentID, i, projectID, siblingID,
		); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFeatureSiblingsTx(tx *sql.Tx, projectID, parentID string) error {
	ids, err := siblingIDsTx(tx, projectID, parentID, "")
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.Exec(
			`UPDATE feature_nodes SET position = ? WHERE project_id = ? AND id = ?`,
			i, projectID, id,
		); err != nil {
			return err
		}
	}
	return nil
}

func featureNodeSnapshot(node FeatureNode) map[string]any {
	return map[string]any{
		"id": node.ID, "parentId": node.ParentID, "kind": node.Kind,
		"title": node.Title, "description": node.Description,
		"documents":         node.Documents,
		"targetMilestoneId": node.TargetMilestoneID, "position": node.Position,
	}
}

func appendFeatureEventTx(tx *sql.Tx, event ProjectEvent, projectID, targetType, targetID, operation string, before, after any) error {
	event.ProjectID = projectID
	event.TargetType = targetType
	event.TargetID = targetID
	event.Operation = operation
	event.EventType = targetType + "." + operation
	event.Before, _ = json.Marshal(before)
	event.After, _ = json.Marshal(after)
	_, err := appendProjectEventTx(tx, event, false)
	return err
}

func createFeatureNodeTx(tx *sql.Tx, node FeatureNode, event ProjectEvent) (FeatureNode, error) {
	node.Title = strings.TrimSpace(node.Title)
	if node.ProjectID == "" || node.Title == "" {
		return FeatureNode{}, ErrFeatureInvalidParent
	}
	if err := ensureFeatureProjectTx(tx, node.ProjectID); err != nil {
		return FeatureNode{}, err
	}
	if err := validateFeaturePlacementTx(tx, node, node.ParentID); err != nil {
		return FeatureNode{}, err
	}
	if node.TargetMilestoneID != "" {
		if node.Kind != FeatureNodePoint {
			return FeatureNode{}, ErrFeatureInvalidKind
		}
		if _, err := validateFeatureMilestoneTx(tx, node.ProjectID, node.TargetMilestoneID); err != nil {
			return FeatureNode{}, err
		}
	}
	if node.ID == "" {
		node.ID = newID()
	}
	now := time.Now().UTC()
	node.CreatedAt = now
	node.UpdatedAt = now
	documentsJSON, err := featureDocumentsJSON(node.Documents)
	if err != nil {
		return FeatureNode{}, err
	}
	if _, err := tx.Exec(`
		INSERT INTO feature_nodes (
			id, project_id, parent_id, kind, title, description,
			documents_json, target_milestone_id, position, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, -1, ?, ?)`,
		node.ID, node.ProjectID, node.ParentID, node.Kind, node.Title,
		node.Description, documentsJSON, node.TargetMilestoneID, timeToStr(now), timeToStr(now),
	); err != nil {
		return FeatureNode{}, err
	}
	if err := placeFeatureNodeTx(tx, node.ProjectID, node.ParentID, node.ID, node.Position); err != nil {
		return FeatureNode{}, err
	}
	stored, err := featureNodeTx(tx, node.ID)
	if err != nil {
		return FeatureNode{}, err
	}
	if err := appendFeatureEventTx(
		tx, event, node.ProjectID, "feature_node", node.ID, "create",
		map[string]any{}, featureNodeSnapshot(stored),
	); err != nil {
		return FeatureNode{}, err
	}
	return stored, nil
}

func updateFeatureNodeTx(tx *sql.Tx, projectID, id string, patch FeatureNodePatch, event ProjectEvent) (FeatureNode, error) {
	current, err := featureNodeTx(tx, id)
	if err != nil {
		return FeatureNode{}, err
	}
	if current.ProjectID != projectID {
		return FeatureNode{}, ErrProjectMismatch
	}
	before := current
	if patch.Title != nil {
		current.Title = strings.TrimSpace(*patch.Title)
		if current.Title == "" {
			return FeatureNode{}, ErrFeatureInvalidParent
		}
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	if patch.Documents != nil {
		current.Documents = normalizeFeatureDocuments(*patch.Documents)
	}
	if patch.TargetMilestoneID != nil {
		if current.Kind != FeatureNodePoint && *patch.TargetMilestoneID != "" {
			return FeatureNode{}, ErrFeatureInvalidKind
		}
		current.TargetMilestoneID = *patch.TargetMilestoneID
		if current.TargetMilestoneID != "" {
			if _, err := validateFeatureMilestoneTx(tx, projectID, current.TargetMilestoneID); err != nil {
				return FeatureNode{}, err
			}
		}
	}
	targetParent := current.ParentID
	targetPosition := current.Position
	if patch.ParentID != nil {
		targetParent = *patch.ParentID
	}
	if patch.Position != nil {
		targetPosition = *patch.Position
	}
	moving := targetParent != before.ParentID || targetPosition != before.Position
	if moving {
		if err := validateFeaturePlacementTx(tx, current, targetParent); err != nil {
			return FeatureNode{}, err
		}
	}
	current.UpdatedAt = time.Now().UTC()
	documentsJSON, err := featureDocumentsJSON(current.Documents)
	if err != nil {
		return FeatureNode{}, err
	}
	if _, err := tx.Exec(`
		UPDATE feature_nodes SET title = ?, description = ?, documents_json = ?,
			target_milestone_id = ?, updated_at = ?
		WHERE project_id = ? AND id = ?`,
		current.Title, current.Description, documentsJSON, current.TargetMilestoneID,
		timeToStr(current.UpdatedAt), projectID, id,
	); err != nil {
		return FeatureNode{}, err
	}
	if moving {
		if _, err := tx.Exec(
			`UPDATE feature_nodes SET parent_id = ?, position = -1 WHERE project_id = ? AND id = ?`,
			targetParent, projectID, id,
		); err != nil {
			return FeatureNode{}, err
		}
		if before.ParentID != targetParent {
			if err := normalizeFeatureSiblingsTx(tx, projectID, before.ParentID); err != nil {
				return FeatureNode{}, err
			}
		}
		if err := placeFeatureNodeTx(tx, projectID, targetParent, id, targetPosition); err != nil {
			return FeatureNode{}, err
		}
	}
	stored, err := featureNodeTx(tx, id)
	if err != nil {
		return FeatureNode{}, err
	}
	operation := "update"
	if moving {
		operation = "move"
	}
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_node", id, operation,
		featureNodeSnapshot(before), featureNodeSnapshot(stored),
	); err != nil {
		return FeatureNode{}, err
	}
	return stored, nil
}

func (s *FeatureCatalogStore) Create(node FeatureNode, event ProjectEvent) (FeatureNode, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return FeatureNode{}, err
	}
	defer tx.Rollback()
	stored, err := createFeatureNodeTx(tx, node, event)
	if err != nil {
		return FeatureNode{}, err
	}
	return stored, tx.Commit()
}

func (s *FeatureCatalogStore) Update(projectID, id string, patch FeatureNodePatch, event ProjectEvent) (FeatureNode, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return FeatureNode{}, err
	}
	defer tx.Rollback()
	stored, err := updateFeatureNodeTx(tx, projectID, id, patch, event)
	if err != nil {
		return FeatureNode{}, err
	}
	return stored, tx.Commit()
}

func (s *FeatureCatalogStore) Delete(projectID, id string, event ProjectEvent) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	node, err := featureNodeTx(tx, id)
	if err != nil {
		return err
	}
	if node.ProjectID != projectID {
		return ErrProjectMismatch
	}
	var children int
	if err := tx.QueryRow(
		`SELECT COUNT(1) FROM feature_nodes WHERE project_id = ? AND parent_id = ?`,
		projectID, id,
	).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return ErrFeatureHasChildren
	}
	if _, err := tx.Exec(`DELETE FROM feature_item_links WHERE feature_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM feature_nodes WHERE project_id = ? AND id = ?`, projectID, id); err != nil {
		return err
	}
	if err := normalizeFeatureSiblingsTx(tx, projectID, node.ParentID); err != nil {
		return err
	}
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_node", id, "delete",
		featureNodeSnapshot(node), map[string]any{},
	); err != nil {
		return err
	}
	return tx.Commit()
}

func validateFeatureLink(relation FeatureItemRelation, itemType ItemType) error {
	switch relation {
	case FeatureItemSource:
		if itemType != ItemTypeRequirement && itemType != ItemTypeBug {
			return ErrFeatureInvalidItemType
		}
	case FeatureItemDelivery:
		if itemType != ItemTypeTask && itemType != "" {
			return ErrFeatureInvalidItemType
		}
	default:
		return ErrFeatureInvalidRelation
	}
	return nil
}

func linkFeatureItemTx(tx *sql.Tx, projectID, featureID, itemID string, relation FeatureItemRelation, event ProjectEvent) (FeatureItemLink, bool, error) {
	node, err := featureNodeTx(tx, featureID)
	if err != nil {
		return FeatureItemLink{}, false, err
	}
	if node.ProjectID != projectID {
		return FeatureItemLink{}, false, ErrProjectMismatch
	}
	if node.Kind != FeatureNodePoint {
		return FeatureItemLink{}, false, ErrFeatureInvalidKind
	}
	var itemProject string
	var itemType ItemType
	if err := tx.QueryRow(
		`SELECT project_id, type FROM project_items WHERE id = ?`, itemID,
	).Scan(&itemProject, &itemType); err != nil {
		if err == sql.ErrNoRows {
			return FeatureItemLink{}, false, ErrNotFound
		}
		return FeatureItemLink{}, false, err
	}
	if itemProject != projectID {
		return FeatureItemLink{}, false, ErrProjectMismatch
	}
	if err := validateFeatureLink(relation, itemType); err != nil {
		return FeatureItemLink{}, false, err
	}
	var exists int
	if err := tx.QueryRow(`
		SELECT COUNT(1) FROM feature_item_links
		WHERE feature_id = ? AND item_id = ? AND relation = ?`,
		featureID, itemID, relation,
	).Scan(&exists); err != nil {
		return FeatureItemLink{}, false, err
	}
	if exists > 0 {
		var createdAt string
		if err := tx.QueryRow(`
			SELECT created_at FROM feature_item_links
			WHERE feature_id = ? AND item_id = ? AND relation = ?`,
			featureID, itemID, relation,
		).Scan(&createdAt); err != nil {
			return FeatureItemLink{}, false, err
		}
		return FeatureItemLink{
			FeatureID: featureID, ItemID: itemID, Relation: relation,
			CreatedAt: strToTime(createdAt),
		}, false, nil
	}
	link := FeatureItemLink{
		FeatureID: featureID, ItemID: itemID, Relation: relation,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := tx.Exec(`
		INSERT INTO feature_item_links (feature_id, item_id, relation, created_at)
		VALUES (?, ?, ?, ?)`,
		featureID, itemID, relation, timeToStr(link.CreatedAt),
	); err != nil {
		return FeatureItemLink{}, false, err
	}
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_link",
		fmt.Sprintf("%s:%s:%s", featureID, itemID, relation), "link",
		map[string]any{}, link,
	); err != nil {
		return FeatureItemLink{}, false, err
	}
	return link, true, nil
}

func unlinkFeatureItemTx(tx *sql.Tx, projectID, featureID, itemID string, relation FeatureItemRelation, event ProjectEvent) (bool, error) {
	if relation != FeatureItemSource && relation != FeatureItemDelivery {
		return false, ErrFeatureInvalidRelation
	}
	node, err := featureNodeTx(tx, featureID)
	if err != nil {
		return false, err
	}
	if node.ProjectID != projectID {
		return false, ErrProjectMismatch
	}
	if node.Kind != FeatureNodePoint {
		return false, ErrFeatureInvalidKind
	}
	result, err := tx.Exec(`
		DELETE FROM feature_item_links
		WHERE feature_id = ? AND item_id = ? AND relation = ?`,
		featureID, itemID, relation,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	link := FeatureItemLink{FeatureID: featureID, ItemID: itemID, Relation: relation}
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_link",
		fmt.Sprintf("%s:%s:%s", featureID, itemID, relation), "unlink",
		link, map[string]any{},
	); err != nil {
		return false, err
	}
	return true, nil
}

func (s *FeatureCatalogStore) LinkItem(projectID, featureID, itemID string, relation FeatureItemRelation, event ProjectEvent) (FeatureItemLink, bool, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return FeatureItemLink{}, false, err
	}
	defer tx.Rollback()
	link, created, err := linkFeatureItemTx(tx, projectID, featureID, itemID, relation, event)
	if err != nil {
		return FeatureItemLink{}, false, err
	}
	return link, created, tx.Commit()
}

func (s *FeatureCatalogStore) UnlinkItem(projectID, featureID, itemID string, relation FeatureItemRelation, event ProjectEvent) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := unlinkFeatureItemTx(tx, projectID, featureID, itemID, relation, event); err != nil {
		return err
	}
	return tx.Commit()
}

func resolveFeatureBatchRef(refs map[string]string, direct, ref string) (string, error) {
	if ref == "" {
		return direct, nil
	}
	if direct != "" {
		return "", fmt.Errorf("%w: direct id and client reference are mutually exclusive", ErrFeatureInvalidParent)
	}
	id, ok := refs[ref]
	if !ok {
		return "", fmt.Errorf("%w: unknown client reference %q", ErrFeatureInvalidParent, ref)
	}
	return id, nil
}

// Batch validates and applies every operation in one database transaction.
// Writes are visible to later validation inside the transaction, but none are
// visible outside it until the single commit succeeds.
func (s *FeatureCatalogStore) Batch(projectID string, operations []FeatureCatalogBatchOperation, baseEvent ProjectEvent) ([]FeatureCatalogBatchResult, error) {
	if projectID == "" || len(operations) == 0 {
		return nil, ErrFeatureInvalidParent
	}
	tx, err := s.db.sql.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := ensureFeatureProjectTx(tx, projectID); err != nil {
		return nil, err
	}

	refs := map[string]string{}
	results := make([]FeatureCatalogBatchResult, 0, len(operations))
	for i, op := range operations {
		operation := strings.ToLower(strings.TrimSpace(op.Operation))
		event := baseEvent
		event.ID = newID()
		var result FeatureCatalogBatchResult
		result.Operation = operation

		switch operation {
		case "create":
			if op.ClientRef != "" {
				if _, exists := refs[op.ClientRef]; exists {
					return nil, fmt.Errorf("%w: duplicate clientRef %q", ErrFeatureInvalidParent, op.ClientRef)
				}
			}
			parentID := ""
			if op.ParentID != nil {
				parentID = *op.ParentID
			}
			parentID, err = resolveFeatureBatchRef(refs, parentID, op.ParentRef)
			if err != nil {
				return nil, fmt.Errorf("operation %d: %w", i, err)
			}
			position := 0
			if op.Position != nil {
				position = *op.Position
			}
			title := ""
			if op.Title != nil {
				title = *op.Title
			}
			description := ""
			if op.Description != nil {
				description = *op.Description
			}
			documents := []string{}
			if op.Documents != nil {
				documents = *op.Documents
			}
			targetMilestoneID := ""
			if op.TargetMilestoneID != nil {
				targetMilestoneID = *op.TargetMilestoneID
			}
			node, createErr := createFeatureNodeTx(tx, FeatureNode{
				ProjectID: projectID, ParentID: parentID, Kind: op.Kind,
				Title: title, Description: description, Documents: documents,
				TargetMilestoneID: targetMilestoneID, Position: position,
			}, event)
			if createErr != nil {
				return nil, fmt.Errorf("operation %d: %w", i, createErr)
			}
			if op.ClientRef != "" {
				refs[op.ClientRef] = node.ID
			}
			result.ClientRef = op.ClientRef
			result.Node = &node

		case "update", "move":
			id, resolveErr := resolveFeatureBatchRef(refs, op.ID, op.NodeRef)
			if resolveErr != nil {
				return nil, fmt.Errorf("operation %d: %w", i, resolveErr)
			}
			if id == "" {
				return nil, fmt.Errorf("operation %d: %w", i, ErrNotFound)
			}
			parentID := op.ParentID
			if op.ParentRef != "" {
				resolved, refErr := resolveFeatureBatchRef(refs, "", op.ParentRef)
				if refErr != nil {
					return nil, fmt.Errorf("operation %d: %w", i, refErr)
				}
				parentID = &resolved
			}
			patch := FeatureNodePatch{
				ParentID: parentID, Title: op.Title, Description: op.Description,
				Documents: op.Documents, TargetMilestoneID: op.TargetMilestoneID,
				Position: op.Position,
			}
			if operation == "move" && patch.ParentID == nil && patch.Position == nil {
				return nil, fmt.Errorf("operation %d: %w", i, ErrFeatureInvalidParent)
			}
			node, updateErr := updateFeatureNodeTx(tx, projectID, id, patch, event)
			if updateErr != nil {
				return nil, fmt.Errorf("operation %d: %w", i, updateErr)
			}
			result.Node = &node

		case "link":
			featureID, resolveErr := resolveFeatureBatchRef(refs, op.FeatureID, op.FeatureRef)
			if resolveErr != nil {
				return nil, fmt.Errorf("operation %d: %w", i, resolveErr)
			}
			link, created, linkErr := linkFeatureItemTx(
				tx, projectID, featureID, op.ItemID, op.Relation, event,
			)
			if linkErr != nil {
				return nil, fmt.Errorf("operation %d: %w", i, linkErr)
			}
			result.Link = &link
			result.Created = &created

		case "unlink":
			featureID, resolveErr := resolveFeatureBatchRef(refs, op.FeatureID, op.FeatureRef)
			if resolveErr != nil {
				return nil, fmt.Errorf("operation %d: %w", i, resolveErr)
			}
			if _, unlinkErr := unlinkFeatureItemTx(
				tx, projectID, featureID, op.ItemID, op.Relation, event,
			); unlinkErr != nil {
				return nil, fmt.Errorf("operation %d: %w", i, unlinkErr)
			}
			result.OK = true

		default:
			return nil, fmt.Errorf("operation %d: %w: unsupported op %q", i, ErrFeatureInvalidParent, op.Operation)
		}
		results = append(results, result)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

type featureMilestoneQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

func featureMilestonePreview(
	q featureMilestoneQueryer,
	projectID, featureID string,
) (FeatureMilestoneSyncPreview, error) {
	node, err := scanFeatureNode(q.QueryRow(
		`SELECT `+featureNodeCols+` FROM feature_nodes WHERE id = ?`, featureID,
	))
	if err == sql.ErrNoRows {
		return FeatureMilestoneSyncPreview{}, ErrNotFound
	}
	if err != nil {
		return FeatureMilestoneSyncPreview{}, err
	}
	if node.ProjectID != projectID {
		return FeatureMilestoneSyncPreview{}, ErrProjectMismatch
	}
	if node.Kind != FeatureNodePoint {
		return FeatureMilestoneSyncPreview{}, ErrFeatureInvalidKind
	}

	preview := FeatureMilestoneSyncPreview{
		FeatureID:         featureID,
		TargetMilestoneID: node.TargetMilestoneID,
		Tasks:             []FeatureMilestoneTaskDiff{},
	}
	if node.TargetMilestoneID != "" {
		var milestoneProject string
		if err := q.QueryRow(`
			SELECT project_id, name, version FROM milestones WHERE id = ?`,
			node.TargetMilestoneID,
		).Scan(&milestoneProject, &preview.TargetMilestone, &preview.TargetVersion); err != nil {
			if err == sql.ErrNoRows {
				return FeatureMilestoneSyncPreview{}, ErrNotFound
			}
			return FeatureMilestoneSyncPreview{}, err
		}
		if milestoneProject != projectID {
			return FeatureMilestoneSyncPreview{}, ErrProjectMismatch
		}
		if preview.TargetVersion == "" {
			return FeatureMilestoneSyncPreview{}, ErrFeatureInvalidMilestone
		}
	}

	rows, err := q.Query(`
		SELECT i.id, i.number, i.title, i.milestone
		FROM feature_item_links l
		JOIN project_items i ON i.id = l.item_id
		WHERE l.feature_id = ? AND l.relation = 'delivery'
			AND i.project_id = ? AND i.milestone != ?
		ORDER BY i.number, i.created_at, i.id`,
		featureID, projectID, preview.TargetMilestone,
	)
	if err != nil {
		return FeatureMilestoneSyncPreview{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var task FeatureMilestoneTaskDiff
		if err := rows.Scan(&task.ID, &task.Number, &task.Title, &task.CurrentMilestone); err != nil {
			return FeatureMilestoneSyncPreview{}, err
		}
		preview.Tasks = append(preview.Tasks, task)
	}
	return preview, rows.Err()
}

// PreviewMilestoneSync lists delivery tasks that differ from a feature point's
// target. It does not mutate either the feature or its tasks.
func (s *FeatureCatalogStore) PreviewMilestoneSync(
	projectID, featureID string,
) (FeatureMilestoneSyncPreview, error) {
	return featureMilestonePreview(s.db.sql, projectID, featureID)
}

// TaskMilestone resolves the milestone label inherited by a task created from
// a feature point. The empty string is a valid result for an unversioned point.
func (s *FeatureCatalogStore) TaskMilestone(projectID, featureID string) (string, error) {
	preview, err := s.PreviewMilestoneSync(projectID, featureID)
	if err != nil {
		return "", err
	}
	return preview.TargetMilestone, nil
}

// SyncMilestone updates every mismatched delivery task only after an explicit
// caller action. The preview and task updates share one transaction.
func (s *FeatureCatalogStore) SyncMilestone(
	projectID, featureID string,
	event ProjectEvent,
) (FeatureMilestoneSyncPreview, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return FeatureMilestoneSyncPreview{}, err
	}
	defer tx.Rollback()
	preview, err := featureMilestonePreview(tx, projectID, featureID)
	if err != nil {
		return FeatureMilestoneSyncPreview{}, err
	}
	if len(preview.Tasks) == 0 {
		return preview, tx.Commit()
	}
	now := timeToStr(time.Now().UTC())
	if _, err := tx.Exec(`
		UPDATE project_items SET milestone = ?, updated_at = ?
		WHERE project_id = ? AND id IN (
			SELECT item_id FROM feature_item_links
			WHERE feature_id = ? AND relation = 'delivery'
		) AND milestone != ?`,
		preview.TargetMilestone, now, projectID, featureID, preview.TargetMilestone,
	); err != nil {
		return FeatureMilestoneSyncPreview{}, err
	}
	if err := appendFeatureEventTx(
		tx, event, projectID, "feature_milestone", featureID, "sync",
		map[string]any{"tasks": preview.Tasks},
		map[string]any{
			"targetMilestoneId": preview.TargetMilestoneID,
			"targetMilestone":   preview.TargetMilestone,
			"targetVersion":     preview.TargetVersion,
		},
	); err != nil {
		return FeatureMilestoneSyncPreview{}, err
	}
	return preview, tx.Commit()
}

func (s *FeatureCatalogStore) GanttView(projectID string) (GanttData, error) {
	// 1 & 2: Load feature catalog
	catalog, err := s.List(projectID)
	if err != nil {
		return GanttData{}, err
	}

	// 3: Load tasks
	taskRows, err := s.db.sql.Query(`
		SELECT id, title, status, planned_start, planned_end, milestone, number
		FROM project_items
		WHERE project_id = ? AND (type = '' OR type = 'task')`, projectID)
	if err != nil {
		return GanttData{}, err
	}
	defer taskRows.Close()

	tasks := map[string]GanttTaskEntry{}
	for taskRows.Next() {
		var task GanttTaskEntry
		var plannedStart, plannedEnd sql.NullString
		if err := taskRows.Scan(
			&task.ID, &task.Title, &task.Status,
			&plannedStart, &plannedEnd, &task.Milestone, &task.Number,
		); err != nil {
			return GanttData{}, err
		}
		if plannedStart.Valid {
			t := strToTime(plannedStart.String)
			task.PlannedStart = &t
		}
		if plannedEnd.Valid {
			t := strToTime(plannedEnd.String)
			task.PlannedEnd = &t
		}
		task.DependsOn = []string{}
		if task.Status == TaskStatusCompleted {
			task.Progress = 100
		}
		tasks[task.ID] = task
	}
	if err := taskRows.Err(); err != nil {
		return GanttData{}, err
	}
	dependencies, err := s.projectDependencies(projectID)
	if err != nil {
		return GanttData{}, err
	}
	for taskID, dependsOn := range dependencies {
		task, ok := tasks[taskID]
		if !ok {
			continue
		}
		task.DependsOn = dependsOn
		tasks[taskID] = task
	}

	// 4: Load milestones
	msRows, err := s.db.sql.Query(`
		SELECT id, name, version, target_date
		FROM milestones
		WHERE project_id = ? AND version != ''
		ORDER BY position, created_at, id`, projectID)
	if err != nil {
		return GanttData{}, err
	}
	defer msRows.Close()

	milestones := []GanttMilestone{}
	for msRows.Next() {
		var ms GanttMilestone
		var targetDate sql.NullString
		if err := msRows.Scan(&ms.ID, &ms.Name, &ms.Version, &targetDate); err != nil {
			return GanttData{}, err
		}
		if targetDate.Valid {
			t := strToTime(targetDate.String)
			ms.TargetDate = &t
		}
		milestones = append(milestones, ms)
	}

	// 5-9: Build Tree
	nodesByID := map[string]*FeatureNode{}
	for i := range catalog.Nodes {
		nodesByID[catalog.Nodes[i].ID] = &catalog.Nodes[i]
	}

	// Group delivery tasks by feature ID
	deliveries := map[string][]string{}
	for _, link := range catalog.Links {
		if link.Relation == FeatureItemDelivery {
			deliveries[link.FeatureID] = append(deliveries[link.FeatureID], link.ItemID)
		}
	}

	rootModules := []GanttModule{}

	// Helper to find path and depth
	var getPath func(nodeID string) ([]string, int)
	getPath = func(nodeID string) ([]string, int) {
		node, ok := nodesByID[nodeID]
		if !ok || node.ParentID == "" {
			return []string{}, 0
		}
		parentPath, depth := getPath(node.ParentID)
		if pNode, pok := nodesByID[node.ParentID]; pok {
			parentPath = append(parentPath, pNode.Title)
		}
		return parentPath, depth + 1
	}

	// Build modules
	modulesByID := map[string]*GanttModule{}
	for _, node := range catalog.Nodes {
		if node.Kind == FeatureNodeModule {
			path, depth := getPath(node.ID)
			mod := &GanttModule{
				ID:       node.ID,
				Title:    node.Title,
				Path:     path,
				Depth:    depth,
				Children: []GanttModule{},
				Tasks:    []GanttTaskEntry{},
			}
			modulesByID[node.ID] = mod
		}
	}

	// Assign tasks to modules
	scheduledTasks := map[string]bool{}
	for _, node := range catalog.Nodes {
		if node.Kind == FeatureNodePoint && node.ParentID != "" {
			if mod, ok := modulesByID[node.ParentID]; ok {
				for _, taskID := range deliveries[node.ID] {
					if task, exists := tasks[taskID]; exists {
						mod.Tasks = append(mod.Tasks, task)
						scheduledTasks[taskID] = true
					}
				}
			}
		}
	}

	// Build a module adjacency list in catalog order. GanttModule contains
	// value slices, so copying children into parents before their descendants
	// are attached would truncate a three-level tree.
	moduleChildren := map[string][]string{}
	for _, node := range catalog.Nodes {
		if node.Kind == FeatureNodeModule && node.ParentID != "" {
			if _, ok := modulesByID[node.ParentID]; ok {
				moduleChildren[node.ParentID] = append(moduleChildren[node.ParentID], node.ID)
			}
		}
	}

	var buildModule func(string) GanttModule
	buildModule = func(id string) GanttModule {
		mod := *modulesByID[id]
		for _, childID := range moduleChildren[id] {
			mod.Children = append(mod.Children, buildModule(childID))
		}
		return mod
	}

	// Aggregate dates and progress up the tree
	var aggregate func(mod *GanttModule) (int, int)
	aggregate = func(mod *GanttModule) (int, int) {
		total := len(mod.Tasks)
		completed := 0

		for _, task := range mod.Tasks {
			if task.Status == TaskStatusCompleted {
				completed++
			}
			if task.PlannedStart != nil {
				if mod.AggStart == nil || task.PlannedStart.Before(*mod.AggStart) {
					mod.AggStart = task.PlannedStart
				}
			}
			if task.PlannedEnd != nil {
				if mod.AggEnd == nil || task.PlannedEnd.After(*mod.AggEnd) {
					mod.AggEnd = task.PlannedEnd
				}
			}
		}

		for i := range mod.Children {
			childTotal, childCompleted := aggregate(&mod.Children[i])
			total += childTotal
			completed += childCompleted

			if mod.Children[i].AggStart != nil {
				if mod.AggStart == nil || mod.Children[i].AggStart.Before(*mod.AggStart) {
					mod.AggStart = mod.Children[i].AggStart
				}
			}
			if mod.Children[i].AggEnd != nil {
				if mod.AggEnd == nil || mod.Children[i].AggEnd.After(*mod.AggEnd) {
					mod.AggEnd = mod.Children[i].AggEnd
				}
			}
		}

		if total > 0 {
			mod.Progress = float64(completed) * 100 / float64(total)
		}
		return total, completed
	}

	for _, node := range catalog.Nodes {
		if node.Kind == FeatureNodeModule && node.ParentID == "" {
			mod := buildModule(node.ID)
			aggregate(&mod)
			rootModules = append(rootModules, mod)
		}
	}

	unscheduled := []GanttTaskEntry{}
	for taskID, task := range tasks {
		if !scheduledTasks[taskID] || task.PlannedStart == nil || task.PlannedEnd == nil {
			unscheduled = append(unscheduled, task)
		}
	}
	sort.Slice(unscheduled, func(i, j int) bool {
		return unscheduled[i].Number < unscheduled[j].Number
	})

	return GanttData{
		Modules:     rootModules,
		Unscheduled: unscheduled,
		Milestones:  milestones,
	}, nil
}

func (s *FeatureCatalogStore) ExportJSON(projectID string) ([]byte, error) {
	catalog, err := s.List(projectID)
	if err != nil {
		return nil, err
	}
	gantt, err := s.GanttView(projectID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.sql.Query(`
		SELECT id, number, title, type, status, planned_start, planned_end, milestone
		FROM project_items
		WHERE project_id = ?
		ORDER BY number, created_at, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []FeatureCatalogExportItem{}
	for rows.Next() {
		var item FeatureCatalogExportItem
		var plannedStart, plannedEnd sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Number, &item.Title, &item.Type, &item.Status,
			&plannedStart, &plannedEnd, &item.Milestone,
		); err != nil {
			return nil, err
		}
		if item.Type == "" {
			item.Type = ItemTypeTask
		}
		if plannedStart.Valid {
			t := strToTime(plannedStart.String)
			item.PlannedStart = &t
		}
		if plannedEnd.Valid {
			t := strToTime(plannedEnd.String)
			item.PlannedEnd = &t
		}
		item.DependsOn = []string{}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	dependencies, err := s.projectDependencies(projectID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if dependsOn, ok := dependencies[items[i].ID]; ok {
			items[i].DependsOn = dependsOn
		}
	}

	data := FeatureCatalogExportData{
		SchemaVersion: 1,
		Catalog:       catalog,
		Items:         items,
		Gantt:         gantt,
	}
	return json.MarshalIndent(data, "", "  ")
}

func (s *FeatureCatalogStore) projectDependencies(projectID string) (map[string][]string, error) {
	rows, err := s.db.sql.Query(`
		SELECT d.task_id, d.depends_on
		FROM task_deps d
		JOIN project_items t ON t.id = d.task_id
		WHERE t.project_id = ?
		ORDER BY d.task_id, d.seq`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dependencies := map[string][]string{}
	for rows.Next() {
		var taskID, dependsOn string
		if err := rows.Scan(&taskID, &dependsOn); err != nil {
			return nil, err
		}
		dependencies[taskID] = append(dependencies[taskID], dependsOn)
	}
	return dependencies, rows.Err()
}

func (s *FeatureCatalogStore) ExportMarkdown(projectID string) (string, error) {
	catalog, err := s.List(projectID)
	if err != nil {
		return "", err
	}

	taskRows, err := s.db.sql.Query(`
		SELECT id, title, status, number
		FROM project_items
		WHERE project_id = ?`, projectID)
	if err != nil {
		return "", err
	}
	defer taskRows.Close()

	items := map[string]struct {
		ID     string
		Title  string
		Status TaskStatus
		Number int
	}{}
	for taskRows.Next() {
		var item struct {
			ID     string
			Title  string
			Status TaskStatus
			Number int
		}
		if err := taskRows.Scan(&item.ID, &item.Title, &item.Status, &item.Number); err != nil {
			return "", err
		}
		items[item.ID] = item
	}
	if err := taskRows.Err(); err != nil {
		return "", err
	}

	milestoneRows, err := s.db.sql.Query(`
		SELECT id, version FROM milestones
		WHERE project_id = ? AND version != ''`, projectID)
	if err != nil {
		return "", err
	}
	milestoneVersions := map[string]string{}
	for milestoneRows.Next() {
		var id, version string
		if err := milestoneRows.Scan(&id, &version); err != nil {
			milestoneRows.Close()
			return "", err
		}
		milestoneVersions[id] = version
	}
	if err := milestoneRows.Close(); err != nil {
		return "", err
	}
	if err := milestoneRows.Err(); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# Feature Catalog\n\n")

	nodesByID := map[string]*FeatureNode{}
	for i := range catalog.Nodes {
		nodesByID[catalog.Nodes[i].ID] = &catalog.Nodes[i]
	}

	children := map[string][]string{}
	for _, node := range catalog.Nodes {
		children[node.ParentID] = append(children[node.ParentID], node.ID)
	}

	sources := map[string][]string{}
	deliveries := map[string][]string{}
	for _, link := range catalog.Links {
		if link.Relation == FeatureItemSource {
			sources[link.FeatureID] = append(sources[link.FeatureID], link.ItemID)
		} else if link.Relation == FeatureItemDelivery {
			deliveries[link.FeatureID] = append(deliveries[link.FeatureID], link.ItemID)
		}
	}

	var writeNode func(nodeID string, depth int)
	writeNode = func(nodeID string, depth int) {
		node := nodesByID[nodeID]
		if node.Kind == FeatureNodeModule {
			heading := strings.Repeat("#", depth+1)
			sb.WriteString(fmt.Sprintf("%s %s\n\n", heading, node.Title))
			if node.Description != "" {
				sb.WriteString(node.Description + "\n\n")
			}
			if len(node.Documents) > 0 {
				sb.WriteString("**Documents**:\n")
				for _, document := range node.Documents {
					sb.WriteString(fmt.Sprintf("- `%s`\n", document))
				}
				sb.WriteString("\n")
			}
			if node.Progress != nil {
				sb.WriteString(fmt.Sprintf("**Progress**: %d/%d tasks completed\n\n", node.Progress.CompletedTasks, node.Progress.TotalTasks))
			}

			for _, childID := range children[nodeID] {
				writeNode(childID, depth+1)
			}
		} else if node.Kind == FeatureNodePoint {
			sb.WriteString(fmt.Sprintf("- **%s**\n", node.Title))
			if node.Description != "" {
				sb.WriteString(fmt.Sprintf("  - %s\n", node.Description))
			}
			for _, document := range node.Documents {
				sb.WriteString(fmt.Sprintf("  - Document: `%s`\n", document))
			}

			srcs := sources[nodeID]
			if len(srcs) > 0 {
				sb.WriteString("  - Sources: ")
				var parts []string
				for _, sid := range srcs {
					if item, ok := items[sid]; ok {
						parts = append(parts, fmt.Sprintf("#%d %s", item.Number, item.Title))
					}
				}
				sb.WriteString(strings.Join(parts, ", ") + "\n")
			}

			dels := deliveries[nodeID]
			if len(dels) > 0 {
				sb.WriteString("  - Delivery Tasks: ")
				var parts []string
				for _, did := range dels {
					if item, ok := items[did]; ok {
						parts = append(parts, fmt.Sprintf("#%d %s [%s]", item.Number, item.Title, item.Status))
					}
				}
				sb.WriteString(strings.Join(parts, ", ") + "\n")
			}

			if node.TargetMilestoneID != "" {
				version := milestoneVersions[node.TargetMilestoneID]
				if version == "" {
					version = node.TargetMilestoneID
				}
				sb.WriteString(fmt.Sprintf("  - Target Version: %s\n", version))
			}
			if node.Progress != nil {
				sb.WriteString(fmt.Sprintf("  - Status: %s\n", node.Progress.Status))
			}
		}
	}

	for _, id := range children[""] {
		writeNode(id, 1)
	}

	return sb.String(), nil
}
