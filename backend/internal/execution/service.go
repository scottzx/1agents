package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/provider"
)

// Service is the north-facing execution contract. Dispatch is intentionally
// absent from this first data migration: the existing scheduler remains the
// sole dispatcher until its Executor migration lands.
type Service struct {
	repo     *Repository
	profiles *provider.Store
	dispatch func(context.Context, Job) error
}

func NewService(repo *Repository, profiles *provider.Store) *Service {
	return &Service{repo: repo, profiles: profiles}
}

// SetDispatcher connects the execution definition layer to a southbound
// executor. It is deliberately a narrow callback so the service never imports
// ACP or board packages.
func (s *Service) SetDispatcher(dispatch func(context.Context, Job) error) { s.dispatch = dispatch }

func (s *Service) CreateJob(input CreateJobInput) (Job, error) {
	if input.ProjectID == "" || input.WorkItemID == "" {
		return Job{}, fmt.Errorf("execution: projectId and workItemId are required")
	}
	if err := s.repo.ValidateWorkItem(input.ProjectID, input.WorkItemID); err != nil {
		return Job{}, err
	}
	if input.ExecutorKind == "" {
		input.ExecutorKind = "agent"
	}
	if input.MaxAttempts < 1 {
		input.MaxAttempts = 1
	}
	job := Job{ProjectID: input.ProjectID, WorkItemID: input.WorkItemID, BusinessRef: input.BusinessRef, ExecutorKind: input.ExecutorKind,
		ProfileID: input.ProfileID, LegacyAgentType: input.LegacyAgentType, FunctionType: input.FunctionType, Cwd: input.Cwd,
		Capabilities: input.Capabilities, Status: JobStatusActive, TimeoutMinutes: input.TimeoutMinutes, MaxAttempts: input.MaxAttempts}
	switch job.ExecutorKind {
	case "agent":
		if job.ProfileID != "" && job.LegacyAgentType != "" {
			return Job{}, fmt.Errorf("execution: agent job cannot set both profileId and legacyAgentType")
		}
		if job.LegacyAgentType == "deepseek-build" {
			job.ProfileID = provider.DeepSeekBuildProfileID
			job.LegacyAgentType = ""
			job.ProfileSource = ProfileBindingLegacy
		}
		if job.ProfileID == "" && job.LegacyAgentType == "" {
			projectDefault, err := s.repo.ProjectDefaultProfile(job.ProjectID)
			if err != nil {
				return Job{}, err
			}
			if projectDefault != "" {
				job.ProfileID = projectDefault
				job.ProfileSource = ProfileBindingProjectDefault
			} else if s.profiles != nil {
				systemDefault, defaultErr := s.profiles.DefaultProfileID()
				if defaultErr != nil {
					return Job{}, defaultErr
				}
				job.ProfileID = systemDefault
				if systemDefault != "" {
					job.ProfileSource = ProfileBindingSystemDefault
				}
			}
		}
		if job.ProfileID != "" {
			if s.profiles == nil {
				return Job{}, fmt.Errorf("execution: profile store unavailable")
			}
			profile, err := s.profiles.GetProfile(job.ProfileID)
			if err != nil {
				return Job{}, err
			}
			if profile.Status != provider.ProfileStatusActive {
				return Job{}, fmt.Errorf("execution: profile %q is %s", profile.ID, profile.Status)
			}
			if job.ProfileSource == "" {
				job.ProfileSource = ProfileBindingExplicit
			}
		} else if job.LegacyAgentType != "" {
			job.ProfileSource = ProfileBindingLegacy
		} else {
			return Job{}, fmt.Errorf("execution: agent job requires profileId or legacyAgentType")
		}
	case "function":
		if job.FunctionType == "" {
			return Job{}, fmt.Errorf("execution: function job requires functionType")
		}
	case "human":
	default:
		return Job{}, fmt.Errorf("execution: invalid executorKind %q", job.ExecutorKind)
	}
	return s.repo.Create(job)
}

func (s *Service) GetJob(id string) (Job, error) {
	job, ok, err := s.repo.Get(id)
	if err != nil {
		return Job{}, err
	}
	if !ok {
		return Job{}, fmt.Errorf("execution: job not found")
	}
	return job, nil
}

func (s *Service) ListJobs(projectID string) ([]JobDetail, error) {
	jobs, err := s.repo.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	details := make([]JobDetail, 0, len(jobs))
	for _, job := range jobs {
		detail := JobDetail{Job: job}
		trigger, triggerErr := s.repo.TriggerByJob(job.ID)
		if triggerErr == nil {
			detail.Trigger = &trigger
		} else if triggerErr != meta.ErrNotFound {
			return nil, triggerErr
		}
		details = append(details, detail)
	}
	return details, nil
}

func (s *Service) UpdateJob(id string, patch UpdateJobInput) (Job, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return Job{}, err
	}
	changed := false
	if patch.ProfileID != nil {
		job.ProfileID = *patch.ProfileID
		job.LegacyAgentType = ""
		job.ProfileSource = ProfileBindingExplicit
		changed = true
	}
	if patch.LegacyAgentType != nil {
		job.LegacyAgentType = *patch.LegacyAgentType
		job.ProfileID = ""
		job.ProfileSource = ProfileBindingLegacy
		changed = true
	}
	if patch.FunctionType != nil {
		job.FunctionType = *patch.FunctionType
		changed = true
	}
	if patch.Cwd != nil {
		job.Cwd = *patch.Cwd
		changed = true
	}
	if patch.Capabilities != nil {
		job.Capabilities = *patch.Capabilities
		changed = true
	}
	if patch.TimeoutMinutes != nil {
		job.TimeoutMinutes = *patch.TimeoutMinutes
		changed = true
	}
	if patch.MaxAttempts != nil {
		if *patch.MaxAttempts < 1 {
			return Job{}, fmt.Errorf("execution: maxAttempts must be positive")
		}
		job.MaxAttempts = *patch.MaxAttempts
		changed = true
	}
	if !changed {
		return job, nil
	}
	if job.ExecutorKind == "agent" && job.ProfileID != "" {
		profile, err := s.profiles.GetProfile(job.ProfileID)
		if err != nil {
			return Job{}, err
		}
		if profile.Status != provider.ProfileStatusActive {
			return Job{}, fmt.Errorf("execution: profile %q is %s", profile.ID, profile.Status)
		}
	}
	job.Revision++
	return s.repo.Update(job)
}

func (s *Service) UpsertTrigger(id string, spec TriggerSpec) (Trigger, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return Trigger{}, err
	}
	if spec.Kind != TriggerAt && spec.Kind != TriggerRecurrence {
		return Trigger{}, fmt.Errorf("execution: invalid trigger kind %q", spec.Kind)
	}
	if len(spec.Spec) == 0 {
		return Trigger{}, fmt.Errorf("execution: trigger spec is required")
	}
	if spec.MisfirePolicy == "" {
		spec.MisfirePolicy = "skip"
	}
	if spec.MisfirePolicy != "skip" && spec.MisfirePolicy != "run_once" {
		return Trigger{}, fmt.Errorf("execution: invalid misfirePolicy")
	}
	if spec.OverlapPolicy == "" {
		spec.OverlapPolicy = "forbid"
	}
	if spec.OverlapPolicy != "forbid" && spec.OverlapPolicy != "allow" {
		return Trigger{}, fmt.Errorf("execution: invalid overlapPolicy")
	}
	if job.ExecutorKind == "agent" && spec.OverlapPolicy == "allow" {
		return Trigger{}, fmt.Errorf("execution: agent jobs require overlapPolicy=forbid")
	}
	if spec.NextRunAt == nil {
		var decoded struct {
			At           string `json:"at"`
			EveryMinutes int    `json:"everyMinutes"`
		}
		_ = json.Unmarshal(spec.Spec, &decoded)
		if spec.Kind == TriggerAt && decoded.At != "" {
			at, parseErr := time.Parse(time.RFC3339, decoded.At)
			if parseErr != nil {
				return Trigger{}, fmt.Errorf("execution: at trigger requires RFC3339 spec.at")
			}
			spec.NextRunAt = &at
		}
		if spec.Kind == TriggerRecurrence && decoded.EveryMinutes > 0 {
			next := time.Now().UTC().Add(time.Duration(decoded.EveryMinutes) * time.Minute)
			spec.NextRunAt = &next
		}
	}
	if spec.NextRunAt == nil {
		return Trigger{}, fmt.Errorf("execution: trigger requires nextRunAt or a supported spec.at/everyMinutes")
	}
	return s.repo.UpsertTrigger(Trigger{JobID: id, Kind: spec.Kind, Spec: spec.Spec, Timezone: spec.Timezone, MisfirePolicy: spec.MisfirePolicy, OverlapPolicy: spec.OverlapPolicy, Status: TriggerArmed, NextRunAt: spec.NextRunAt})
}

func (s *Service) PauseJob(id string) error      { return s.repo.SetStatus(id, JobStatusPaused) }
func (s *Service) ResumeJob(id string) error     { return s.repo.SetStatus(id, JobStatusActive) }
func (s *Service) ArchiveJob(id string) error    { return s.repo.SetStatus(id, JobStatusArchived) }
func (s *Service) DeleteTrigger(id string) error { return s.repo.DeleteTrigger(id) }

// RunNow asks the registered Executor to start one immediate occurrence. The
// executor owns the concrete TaskRun creation because it owns the actual
// process/session lifecycle.
func (s *Service) RunNow(ctx context.Context, id string) error {
	job, err := s.GetJob(id)
	if err != nil {
		return err
	}
	if job.Status != JobStatusActive {
		return fmt.Errorf("execution: job %q is %s", id, job.Status)
	}
	if s.dispatch == nil {
		return errDispatchNotEnabled{}
	}
	return s.dispatch(ctx, job)
}

func (s *Service) ListRuns(id string) ([]meta.TaskRun, error) {
	if _, err := s.GetJob(id); err != nil {
		return nil, err
	}
	return meta.NewTaskRunStore(s.repo.db).ListByJob(id)
}
