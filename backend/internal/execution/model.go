// Package execution owns the execution definition layer. It deliberately
// contains no ACP, board policy, or credential logic: jobs describe work,
// triggers describe time, and TaskRun records the resulting attempt.
package execution

import (
	"encoding/json"
	"time"
)

type ProfileBindingSource string

const (
	ProfileBindingExplicit       ProfileBindingSource = "explicit"
	ProfileBindingProjectDefault ProfileBindingSource = "project_default"
	ProfileBindingSystemDefault  ProfileBindingSource = "system_default"
	ProfileBindingLegacy         ProfileBindingSource = "legacy"

	JobStatusActive    = "active"
	JobStatusPaused    = "paused"
	JobStatusBlocked   = "blocked"
	JobStatusCompleted = "completed"
	JobStatusArchived  = "archived"

	TriggerAt         = "at"
	TriggerRecurrence = "recurrence"
	TriggerArmed      = "armed"
	TriggerPaused     = "paused"
	TriggerExhausted  = "exhausted"
)

// Job is the durable execution definition for exactly one board work item in
// the first rollout. Runtime credentials and current run state never belong
// here.
type Job struct {
	ID                   string               `json:"id"`
	ProjectID            string               `json:"projectId"`
	WorkItemID           string               `json:"workItemId"`
	BusinessRef          string               `json:"businessRef,omitempty"`
	ExecutorKind         string               `json:"executorKind"`
	ProfileID            string               `json:"profileId,omitempty"`
	ProfileSource        ProfileBindingSource `json:"profileSource,omitempty"`
	LegacyAgentType      string               `json:"legacyAgentType,omitempty"`
	FunctionType         string               `json:"functionType,omitempty"`
	PreambleFunctionType string               `json:"preambleFunctionType,omitempty"`
	Cwd                  string               `json:"cwd,omitempty"`
	Capabilities         []string             `json:"capabilities,omitempty"`
	Status               string               `json:"status"`
	Revision             int                  `json:"revision"`
	TimeoutMinutes       int                  `json:"timeoutMinutes,omitempty"`
	MaxAttempts          int                  `json:"maxAttempts"`
	BlockedCode          string               `json:"blockedCode,omitempty"`
	BlockedReason        string               `json:"blockedReason,omitempty"`
	CreatedAt            time.Time            `json:"createdAt"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}

type Trigger struct {
	ID            string          `json:"id"`
	JobID         string          `json:"jobId"`
	Kind          string          `json:"kind"`
	Spec          json.RawMessage `json:"spec"`
	Timezone      string          `json:"timezone,omitempty"`
	MisfirePolicy string          `json:"misfirePolicy"`
	OverlapPolicy string          `json:"overlapPolicy"`
	Status        string          `json:"status"`
	NextRunAt     *time.Time      `json:"nextRunAt,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// JobDetail is the aggregate read model used by the execution console. A
// trigger belongs to a job but remains optional: jobs without a trigger are
// manual-only.
type JobDetail struct {
	Job
	Trigger *Trigger `json:"trigger,omitempty"`
}

type CreateJobInput struct {
	ProjectID            string   `json:"projectId"`
	WorkItemID           string   `json:"workItemId"`
	BusinessRef          string   `json:"businessRef,omitempty"`
	ExecutorKind         string   `json:"executorKind"`
	ProfileID            string   `json:"profileId,omitempty"`
	LegacyAgentType      string   `json:"legacyAgentType,omitempty"`
	FunctionType         string   `json:"functionType,omitempty"`
	PreambleFunctionType string   `json:"preambleFunctionType,omitempty"`
	Cwd                  string   `json:"cwd,omitempty"`
	Capabilities         []string `json:"capabilities,omitempty"`
	TimeoutMinutes       int      `json:"timeoutMinutes,omitempty"`
	MaxAttempts          int      `json:"maxAttempts,omitempty"`
}

type UpdateJobInput struct {
	ProfileID            *string   `json:"profileId,omitempty"`
	LegacyAgentType      *string   `json:"legacyAgentType,omitempty"`
	FunctionType         *string   `json:"functionType,omitempty"`
	PreambleFunctionType *string   `json:"preambleFunctionType,omitempty"`
	Cwd                  *string   `json:"cwd,omitempty"`
	Capabilities         *[]string `json:"capabilities,omitempty"`
	TimeoutMinutes       *int      `json:"timeoutMinutes,omitempty"`
	MaxAttempts          *int      `json:"maxAttempts,omitempty"`
}

type TriggerSpec struct {
	Kind          string          `json:"kind"`
	Spec          json.RawMessage `json:"spec"`
	Timezone      string          `json:"timezone,omitempty"`
	MisfirePolicy string          `json:"misfirePolicy,omitempty"`
	OverlapPolicy string          `json:"overlapPolicy,omitempty"`
	NextRunAt     *time.Time      `json:"nextRunAt,omitempty"`
}
