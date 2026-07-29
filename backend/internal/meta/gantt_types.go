package meta

import "time"

// GanttData is the read model for the Gantt chart view. Module dates are
// derived from descendant tasks and never persisted.
type GanttData struct {
	Modules     []GanttModule    `json:"modules"`
	Unscheduled []GanttTaskEntry `json:"unscheduled"`
	Milestones  []GanttMilestone `json:"milestones"`
}

// FeatureCatalogExportData is the versioned, self-contained JSON export
// contract. Catalog preserves the module/feature hierarchy and traceability
// links, Items supplies the linked requirement/task facts, and Gantt carries
// the derived schedule projection.
type FeatureCatalogExportData struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Catalog       FeatureCatalog             `json:"catalog"`
	Items         []FeatureCatalogExportItem `json:"items"`
	Gantt         GanttData                  `json:"gantt"`
}

// FeatureCatalogExportItem is the stable project-item subset required to
// consume a feature-catalog export without access to the source database.
type FeatureCatalogExportItem struct {
	ID           string     `json:"id"`
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	Type         ItemType   `json:"type"`
	Status       TaskStatus `json:"status"`
	PlannedStart *time.Time `json:"plannedStart,omitempty"`
	PlannedEnd   *time.Time `json:"plannedEnd,omitempty"`
	DependsOn    []string   `json:"dependsOn"`
	Milestone    string     `json:"milestone,omitempty"`
}

// GanttModule represents one module node in the Gantt chart, with dates
// aggregated from its descendant tasks.
type GanttModule struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	Path     []string         `json:"path"`
	Depth    int              `json:"depth"`
	AggStart *time.Time       `json:"aggStart,omitempty"`
	AggEnd   *time.Time       `json:"aggEnd,omitempty"`
	Progress float64          `json:"progress"`
	Children []GanttModule    `json:"children"`
	Tasks    []GanttTaskEntry `json:"tasks"`
}

// GanttTaskEntry is one task bar in the Gantt chart.
type GanttTaskEntry struct {
	ID           string     `json:"id"`
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	PlannedStart *time.Time `json:"plannedStart,omitempty"`
	PlannedEnd   *time.Time `json:"plannedEnd,omitempty"`
	Status       TaskStatus `json:"status"`
	Milestone    string     `json:"milestone,omitempty"`
	DependsOn    []string   `json:"dependsOn"`
	Progress     float64    `json:"progress"`
}

// GanttMilestone is a version marker on the Gantt chart timeline.
type GanttMilestone struct {
	ID         string     `json:"id"`
	Version    string     `json:"version"`
	TargetDate *time.Time `json:"targetDate,omitempty"`
}
