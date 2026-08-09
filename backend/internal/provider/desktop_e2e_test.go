package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDesktopConfigCopiesE2E uses copies of real desktop configuration files.
// It is opt-in because it depends on locally installed CLIs and credentials:
//
//	ONEAGENTS_E2E_HOME=$HOME CC_SWITCH_TEST_DB=$HOME/.cc-switch/cc-switch.db go test ./internal/provider -run TestDesktopConfigCopiesE2E -v
//
// Set ONEAGENTS_E2E_LIVE=1 to also create a 1ACP session and send one minimal
// prompt through each agent. ONEAGENTS_E2E_AGENT limits the run to one agent,
// and ONEAGENTS_E2E_PROVIDER_<AGENT> selects a named provider from the real DB.
// The original files are never modified.
func TestDesktopConfigCopiesE2E(t *testing.T) {
	sourceHome := os.Getenv("ONEAGENTS_E2E_HOME")
	dbPath := os.Getenv("CC_SWITCH_TEST_DB")
	if sourceHome == "" || dbPath == "" {
		t.Skip("ONEAGENTS_E2E_HOME and CC_SWITCH_TEST_DB are required")
	}

	store := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	if err := store.ImportCCSwitch(dbPath); err != nil {
		t.Fatal(err)
	}
	bindings, err := store.ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	bindingByAgent := map[AgentID]AgentBinding{}
	for _, binding := range bindings {
		bindingByAgent[binding.AgentID] = binding
	}

	for _, agentID := range []AgentID{AgentClaude, AgentCodex} {
		agentID := agentID
		t.Run(string(agentID), func(t *testing.T) {
			if filter := os.Getenv("ONEAGENTS_E2E_AGENT"); filter != "" && filter != string(agentID) {
				t.Skipf("ONEAGENTS_E2E_AGENT=%s", filter)
			}
			binding, ok := bindingByAgent[agentID]
			if !ok {
				t.Fatalf("cc-switch has no current %s binding", agentID)
			}
			item, err := store.Get(binding.ProviderID)
			if err != nil {
				t.Fatal(err)
			}
			if providerName := os.Getenv("ONEAGENTS_E2E_PROVIDER_" + strings.ToUpper(string(agentID))); providerName != "" {
				item, binding, err = selectE2EProvider(store, providerName, binding)
				if err != nil {
					t.Fatal(err)
				}
			}
			testHome := t.TempDir()
			paths := desktopConfigPaths(sourceHome, testHome, agentID)
			before := map[string][]byte{}
			for source, target := range paths {
				data, err := os.ReadFile(source)
				if err != nil {
					t.Fatalf("read real config copy source %s: %v", source, err)
				}
				if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, data, 0o600); err != nil {
					t.Fatal(err)
				}
				before[target] = data
			}

			result, err := ApplyAgentBinding(testHome, *item, binding)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Success || len(result.Files) == 0 {
				t.Fatalf("apply result = %#v", result)
			}
			if os.Getenv("ONEAGENTS_E2E_LIVE") == "1" {
				run1ACPLiveCheck(t, testHome, agentID, *item)
			}
			if err := RollbackApply(result); err != nil {
				t.Fatal(err)
			}
			for path, expected := range before {
				actual, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(actual) != string(expected) {
					t.Fatalf("rollback changed copied real config %s", path)
				}
			}
		})
	}
}

func selectE2EProvider(store *Store, name string, binding AgentBinding) (*Provider, AgentBinding, error) {
	items, _, err := store.List()
	if err != nil {
		return nil, binding, err
	}
	for i := range items {
		if items[i].Name != name {
			continue
		}
		binding.ProviderID = items[i].ID
		models, err := store.Models(items[i].ID)
		if err != nil {
			return nil, binding, err
		}
		if len(models) > 0 {
			modelID := models[0].ModelID
			binding.ModelID = modelID
			binding.ModelMapping = map[string]string{"default": modelID}
			if binding.AgentID == AgentClaude {
				binding.ModelMapping["opus"] = modelID
				binding.ModelMapping["sonnet"] = modelID
				binding.ModelMapping["haiku"] = modelID
			}
		}
		return &items[i], binding, nil
	}
	return nil, binding, fmt.Errorf("provider %q not found", name)
}

func desktopConfigPaths(sourceHome, testHome string, agentID AgentID) map[string]string {
	paths := map[string]string{}
	add := func(relative string) {
		paths[filepath.Join(sourceHome, relative)] = filepath.Join(testHome, relative)
	}
	switch agentID {
	case AgentClaude:
		add(filepath.Join(".claude", "settings.json"))
		add(filepath.Join(".claude", "config.json"))
	case AgentCodex:
		add(filepath.Join(".codex", "config.toml"))
		add(filepath.Join(".codex", "auth.json"))
	}
	return paths
}

func run1ACPLiveCheck(t *testing.T, home string, agentID AgentID, item Provider) {
	t.Helper()
	if _, err := exec.LookPath("acpx"); err != nil {
		t.Fatal("acpx is not installed")
	}
	env := withoutEnvKeys(os.Environ(), "HOME", "USERPROFILE")
	env = append(env, "HOME="+home, "USERPROFILE="+home)
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	args := []string{"--cwd", workspace, "--timeout", "120", "--format", "text", string(agentID), "exec", "Reply with OK only."}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "acpx", args...)
	cmd.Dir = workspace
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("1ACP %s failed: %v\n%s", strings.Join(args, " "), err, redactE2EOutput(string(output), item))
	}
}

func withoutEnvKeys(values []string, keys ...string) []string {
	prefixes := make([]string, len(keys))
	for i, key := range keys {
		prefixes[i] = key + "="
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		keep := true
		for _, prefix := range prefixes {
			if strings.HasPrefix(value, prefix) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, value)
		}
	}
	return out
}

func redactE2EOutput(output string, item Provider) string {
	secrets := []string{item.APIKey}
	for _, endpoint := range item.Endpoints {
		secrets = append(secrets, endpoint.APIKey)
		for _, value := range endpoint.Headers {
			secrets = append(secrets, value)
		}
	}
	for _, secret := range secrets {
		if secret != "" {
			output = strings.ReplaceAll(output, secret, "[REDACTED]")
		}
	}
	if len(output) > 2000 {
		output = fmt.Sprintf("...%s", output[len(output)-2000:])
	}
	return output
}
