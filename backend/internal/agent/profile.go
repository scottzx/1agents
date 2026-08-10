package agent

import (
	"encoding/json"
	"fmt"

	"github.com/scottzx/1Agents/backend/internal/provider"
)

// RuntimeLaunch is the private Go→1ACP launch envelope. Credentials are
// deliberately separated from Env so 1ACP can keep them memory-only.
type RuntimeLaunch struct {
	ProfileID            string            `json:"profileId"`
	ProfileRevision      int               `json:"profileRevision"`
	PreviousProfileID    string            `json:"previousProfileId,omitempty"`
	PreviousRevision     int               `json:"previousProfileRevision,omitempty"`
	RuntimeID            string            `json:"runtimeId"`
	Argv                 []string          `json:"argv"`
	Model                string            `json:"model"`
	Env                  map[string]string `json:"env,omitempty"`
	TransientCredentials map[string]string `json:"transientCredentials,omitempty"`
	Snapshot             json.RawMessage   `json:"-"`
}

type resolvedTaskProfile struct {
	profileID string
	launch    *RuntimeLaunch
	snapshot  json.RawMessage
}

func resolveProfile(store *provider.Store, profileID string) (*RuntimeLaunch, json.RawMessage, error) {
	spec, err := store.ResolveProfile(profileID)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := json.Marshal(spec.Snapshot)
	if err != nil {
		return nil, nil, fmt.Errorf("encode resolved profile snapshot: %w", err)
	}
	return &RuntimeLaunch{
		ProfileID:            spec.Snapshot.ProfileID,
		ProfileRevision:      spec.Snapshot.ProfileRevision,
		RuntimeID:            spec.Snapshot.RuntimeID,
		Argv:                 append([]string(nil), spec.Argv...),
		Model:                spec.Model,
		Env:                  cloneStringMap(spec.Env),
		TransientCredentials: cloneStringMap(spec.Credentials),
		Snapshot:             append(json.RawMessage(nil), snapshot...),
	}, snapshot, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func (r *TaskRunner) resolveTaskProfile(workspaceID string, task Task) (resolvedTaskProfile, error) {
	profileID := ""
	if task.TaskTarget != nil {
		profileID = task.TaskTarget.ProfileID
	}
	if profileID == "" && r.tasksStore != nil {
		if project, ok, err := r.tasksStore.DB().GetProject(workspaceID); err != nil {
			return resolvedTaskProfile{}, err
		} else if ok {
			profileID = project.DefaultProfileID
		}
	}
	// One-version compatibility for old assignee=deepseek-build tasks.
	if profileID == "" && task.Assignee == AgentTypeDeepSeekBuild {
		profileID = provider.DeepSeekBuildProfileID
	}
	if profileID == "" && task.Assignee == "" {
		profiles, err := r.providerStore.ListProfiles(false)
		if err != nil {
			return resolvedTaskProfile{}, err
		}
		for _, profile := range profiles {
			if profile.System && profile.Status == provider.ProfileStatusActive {
				profileID = profile.ID
				break
			}
		}
	}
	if profileID == "" {
		return resolvedTaskProfile{}, nil
	}
	launch, snapshot, err := resolveProfile(r.providerStore, profileID)
	if err != nil {
		return resolvedTaskProfile{}, err
	}
	return resolvedTaskProfile{profileID: profileID, launch: launch, snapshot: snapshot}, nil
}
