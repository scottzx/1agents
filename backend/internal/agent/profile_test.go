package agent

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/provider"
)

func TestTaskProfilePriorityAndLegacyResolution(t *testing.T) {
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := provider.NewStore(filepath.Join(t.TempDir(), "providers.json"))
	add := func(id, key string) {
		_, err := store.AddOrUpdate(provider.Provider{
			ID: id, Name: id, APIKey: key, Model: id + "-model", ModelIDs: []string{id + "-model"},
			Endpoints: []provider.ProviderEndpoint{{Family: provider.EndpointFamilyOpenAI, Protocol: "openai_chat", BaseURL: "https://" + id + ".test/v1"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.AddProfile(provider.AgentProfile{ID: id + "-build", Name: id + " build", RuntimeID: provider.GrokBuildRuntimeID, ProviderID: id, ModelID: id + "-model"})
		if err != nil {
			t.Fatal(err)
		}
	}
	add("project", "project-key")
	add("task", "task-key")
	if err := db.EnsureWorkspaceProject(meta.Project{ID: "workspace", Name: "Workspace", WorkspacePath: t.TempDir(), DefaultProfileID: "project-build"}); err != nil {
		t.Fatal(err)
	}
	runner := &TaskRunner{tasksStore: meta.NewTaskStore(db), providerStore: store}
	resolved, err := runner.resolveTaskProfile("workspace", Task{TaskTarget: &meta.TaskTargetSpec{ProfileID: "task-build"}})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.profileID != "task-build" || resolved.launch.TransientCredentials["xai.api_key"] != "task-key" {
		t.Fatalf("explicit task profile did not win: %#v", resolved)
	}
	resolved, err = runner.resolveTaskProfile("workspace", Task{})
	if err != nil || resolved.profileID != "project-build" {
		t.Fatalf("project default profile = %#v, %v", resolved, err)
	}
	systemProfile, err := store.GetProfile(provider.DeepSeekBuildProfileID)
	if err != nil {
		t.Fatal(err)
	}
	systemProfile.ProviderID = "task"
	systemProfile.ModelID = "task-model"
	systemProfile.Status = provider.ProfileStatusActive
	if _, err := store.UpdateProfile(systemProfile.ID, *systemProfile); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureWorkspaceProject(meta.Project{ID: "legacy-workspace", Name: "Legacy", WorkspacePath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	resolved, err = runner.resolveTaskProfile("legacy-workspace", Task{Assignee: AgentTypeDeepSeekBuild})
	if err != nil || resolved.profileID != provider.DeepSeekBuildProfileID {
		t.Fatalf("legacy deepseek-build resolution = %#v, %v", resolved, err)
	}
	resolved, err = runner.resolveTaskProfile("legacy-workspace", Task{})
	if err != nil || resolved.profileID != provider.DeepSeekBuildProfileID {
		t.Fatalf("system default profile = %#v, %v", resolved, err)
	}
}

func TestParallelProfileResolutionDoesNotCrossCredentials(t *testing.T) {
	store := provider.NewStore(filepath.Join(t.TempDir(), "providers.json"))
	for _, id := range []string{"deepseek-custom", "kimi"} {
		_, err := store.AddOrUpdate(provider.Provider{ID: id, Name: id, APIKey: id + "-secret", Model: id + "-model", ModelIDs: []string{id + "-model"}, Endpoints: []provider.ProviderEndpoint{{Family: provider.EndpointFamilyOpenAI, BaseURL: "https://" + id + ".test/v1"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddProfile(provider.AgentProfile{ID: id + "-build", Name: id, RuntimeID: provider.GrokBuildRuntimeID, ProviderID: id, ModelID: id + "-model"}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for _, id := range []string{"deepseek-custom", "kimi"} {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			launch, snapshot, err := resolveProfile(store, id+"-build")
			if err != nil {
				t.Errorf("resolve %s: %v", id, err)
				return
			}
			if launch.TransientCredentials["xai.api_key"] != id+"-secret" || strings.Contains(string(snapshot), "secret") {
				t.Errorf("credential crossed or leaked for %s: %#v %s", id, launch, snapshot)
			}
		}()
	}
	wg.Wait()
}
