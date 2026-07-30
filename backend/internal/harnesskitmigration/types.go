package harnesskitmigration

import (
	"context"
	"io"
	"time"
)

const (
	planVersion        = 1
	migrationID        = "skills-manager-v1"
	markerFileName     = migrationID + ".json"
	inProgressFileName = migrationID + ".in-progress.json"
	lockFileName       = "migration.lock"
)

// Config keeps every filesystem authority explicit so tests and callers never
// need to mutate HOME. The command-line defaults are resolved in DefaultConfig.
type Config struct {
	Home              string
	OneAgentsHome     string
	LegacyDir         string
	HarnessKitDataDir string
	BackupRoot        string
	HarnessKitBinary  string
	HKRunner          func(context.Context, Config) error
	Now               func() time.Time
}

type Plan struct {
	Version           int            `json:"version"`
	MigrationID       string         `json:"migrationId"`
	LegacyDir         string         `json:"legacyDir"`
	HarnessKitDataDir string         `json:"harnessKitDataDir"`
	SourceExists      bool           `json:"sourceExists"`
	SourceFingerprint string         `json:"sourceFingerprint,omitempty"`
	Items             []Item         `json:"items"`
	Conflicts         []Conflict     `json:"conflicts"`
	Losses            []Loss         `json:"losses"`
	Counts            map[string]int `json:"counts"`
	LegacyMetadata    LegacyMetadata `json:"legacyMetadata"`
}

type Item struct {
	Kind        string `json:"kind"`
	Action      string `json:"action"`
	Path        string `json:"path"`
	SourcePath  string `json:"sourcePath,omitempty"`
	LinkTarget  string `json:"linkTarget,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type Conflict struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type Loss struct {
	Kind        string `json:"kind"`
	Path        string `json:"path,omitempty"`
	Name        string `json:"name,omitempty"`
	Disposition string `json:"disposition"`
	Reason      string `json:"reason"`
}

type LegacyMetadata struct {
	SkillManifestEntries int      `json:"skillManifestEntries"`
	AgentManifestEntries int      `json:"agentManifestEntries"`
	SlashCommands        int      `json:"slashCommands"`
	SlashSyncRecords     int      `json:"slashSyncRecords"`
	MCPServers           int      `json:"mcpServers"`
	MCPServerNames       []string `json:"mcpServerNames,omitempty"`
	HistoryPackages      int      `json:"historyPackages"`
	PendingConflicts     int      `json:"pendingConflicts"`
}

type Journal struct {
	Version           int           `json:"version"`
	OperationID       string        `json:"operationId"`
	Phase             string        `json:"phase"`
	StartedAt         time.Time     `json:"startedAt"`
	UpdatedAt         time.Time     `json:"updatedAt"`
	BackupID          string        `json:"backupId"`
	BackupPath        string        `json:"backupPath"`
	BackupComplete    bool          `json:"backupComplete"`
	SourceFingerprint string        `json:"sourceFingerprint"`
	Plan              Plan          `json:"plan"`
	Items             []JournalItem `json:"items"`
	Error             string        `json:"error,omitempty"`
	CompletedAt       *time.Time    `json:"completedAt,omitempty"`
}

type JournalItem struct {
	Item            Item   `json:"item"`
	Status          string `json:"status"`
	PostFingerprint string `json:"postFingerprint,omitempty"`
	Error           string `json:"error,omitempty"`
}

type Marker struct {
	Version           int        `json:"version"`
	MigrationID       string     `json:"migrationId"`
	Mode              string     `json:"mode"`
	SourceFingerprint string     `json:"sourceFingerprint,omitempty"`
	BackupID          string     `json:"backupId,omitempty"`
	CompletedAt       time.Time  `json:"completedAt"`
	RolledBackAt      *time.Time `json:"rolledBackAt,omitempty"`
}

type Result struct {
	Status            string     `json:"status"`
	Mode              string     `json:"mode"`
	BackupID          string     `json:"backupId,omitempty"`
	SourceFingerprint string     `json:"sourceFingerprint,omitempty"`
	Materialized      int        `json:"materialized"`
	Unchanged         int        `json:"unchanged"`
	Conflicts         []Conflict `json:"conflicts,omitempty"`
	LossReportPath    string     `json:"lossReportPath,omitempty"`
	MarkerPath        string     `json:"markerPath,omitempty"`
}

type RollbackReport struct {
	BackupID  string     `json:"backupId"`
	Restored  int        `json:"restored"`
	Unchanged int        `json:"unchanged"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// RunCLI parses only arguments after "1agents migrate harnesskit".
func RunCLI(args []string, stdout, stderr io.Writer) int {
	return runCLI(args, stdout, stderr)
}
