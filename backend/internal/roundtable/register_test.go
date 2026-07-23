package roundtable_test

import (
	"testing"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
	// Side-effect: register agents-roundtable manifest (design §6.3).
	_ "github.com/scottzx/1Agents/backend/internal/roundtable"
)

func TestAgentsRoundtableManifestRegistered(t *testing.T) {
	m, ok := appregistry.Get("agents-roundtable")
	if !ok {
		t.Fatal("agents-roundtable not registered in appregistry")
	}
	if m.Name != "Agents 圆桌" {
		t.Errorf("Name = %q, want Agents 圆桌", m.Name)
	}
	if !m.Enabled {
		t.Error("expected Enabled=true by default (no manual config)")
	}
	if len(m.MountPoints) != 1 {
		t.Fatalf("MountPoints len = %d, want 1", len(m.MountPoints))
	}
	mp := m.MountPoints[0]
	if mp.Type != "l1-page" {
		t.Errorf("mount type = %q, want l1-page", mp.Type)
	}
	if mp.ID != "agents-roundtable" {
		t.Errorf("mount id = %q, want agents-roundtable", mp.ID)
	}
	if mp.View != "AgentsRoundtable" {
		t.Errorf("mount view = %q, want AgentsRoundtable", mp.View)
	}
}
