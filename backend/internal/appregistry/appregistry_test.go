package appregistry_test

import (
	"os"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
)

func TestRegisterAndList(t *testing.T) {
	// Use an isolated temp db so tests don't pollute the real meta.db.
	tmp, err := os.MkdirTemp("", "appregistry-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	t.Setenv("ONEAGENTS_HOME", tmp)

	appregistry.Register(appregistry.AppManifest{
		ID:      "testapp",
		Name:    "Test App",
		Version: "0.1.0",
		Enabled: true,
		MountPoints: []appregistry.MountPoint{
			{Type: "project-tab", ID: "main", Label: "Main", View: "MainTab"},
		},
		TaskTypes:    []string{"testapp.do_thing"},
		DomainTables: []string{"testapp_items"},
	})

	apps := appregistry.List()
	var found bool
	for _, a := range apps {
		if a.ID == "testapp" {
			found = true
			if a.Name != "Test App" {
				t.Errorf("expected name 'Test App', got %q", a.Name)
			}
			if len(a.MountPoints) != 1 {
				t.Errorf("expected 1 mount point, got %d", len(a.MountPoints))
			}
		}
	}
	if !found {
		t.Error("registered app not found in List()")
	}
}

func TestGet(t *testing.T) {
	tmp, err := os.MkdirTemp("", "appregistry-get-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	t.Setenv("ONEAGENTS_HOME", tmp)

	_, ok := appregistry.Get("testapp")
	if !ok {
		t.Error("Get: expected testapp to be registered (from TestRegisterAndList)")
	}

	_, ok = appregistry.Get("nonexistent")
	if ok {
		t.Error("Get: expected false for nonexistent app")
	}
}

func TestSetEnabled(t *testing.T) {
	tmp, err := os.MkdirTemp("", "appregistry-enable-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	t.Setenv("ONEAGENTS_HOME", tmp)

	// testapp is already registered from TestRegisterAndList (same process).
	if err := appregistry.SetEnabled("testapp", false); err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}

	a, ok := appregistry.Get("testapp")
	if !ok {
		t.Fatal("app not found after SetEnabled")
	}
	if a.Enabled {
		t.Error("expected enabled=false after SetEnabled(false)")
	}

	if err := appregistry.SetEnabled("testapp", true); err != nil {
		t.Fatalf("SetEnabled true: %v", err)
	}
	a, _ = appregistry.Get("testapp")
	if !a.Enabled {
		t.Error("expected enabled=true after SetEnabled(true)")
	}
}

func TestSetEnabledUnknownApp(t *testing.T) {
	err := appregistry.SetEnabled("does_not_exist_xyz", true)
	if err == nil {
		t.Error("expected error for unknown app id")
	}
}

func TestEnsureDomainTables(t *testing.T) {
	tmp, err := os.MkdirTemp("", "appregistry-domain-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	t.Setenv("ONEAGENTS_HOME", tmp)

	ddls := []string{
		`CREATE TABLE IF NOT EXISTS testapp_items (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT ''
		)`,
	}
	// First call creates the table.
	if err := appregistry.EnsureDomainTables("testapp", ddls); err != nil {
		t.Fatalf("EnsureDomainTables first call: %v", err)
	}
	// Second call is idempotent — must not error.
	if err := appregistry.EnsureDomainTables("testapp", ddls); err != nil {
		t.Fatalf("EnsureDomainTables second call (idempotent): %v", err)
	}
}

func TestEnsureDomainTablesWrongPrefix(t *testing.T) {
	tmp, err := os.MkdirTemp("", "appregistry-prefix-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	t.Setenv("ONEAGENTS_HOME", tmp)

	// Table name does NOT carry the app prefix — must error.
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS wrong_prefix_items (id TEXT PRIMARY KEY)`,
	}
	if err := appregistry.EnsureDomainTables("testapp", ddls); err == nil {
		t.Error("expected error for table without app prefix")
	}
}
