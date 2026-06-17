package agent

import "testing"

// fixtureCatalog builds a CatalogStore with hand-set statuses, bypassing the
// real PATH probe so the curation logic can be tested deterministically.
func fixtureCatalog(statuses []AgentStatus) *CatalogStore {
	return &CatalogStore{statuses: statuses}
}

func TestCreatableAgents(t *testing.T) {
	statuses := []AgentStatus{
		// integrated + installed CLI-stream → creatable, no command
		{Type: "claudecode", Label: "Claude Code", Installed: true, ChatReady: true, CcTransport: TransportCLIStream, Integrated: true, Path: "/usr/bin/claude"},
		// integrated + installed ACP → creatable, command = detected path
		{Type: "devin", Label: "Devin", Installed: true, ChatReady: true, CcTransport: TransportACP, Integrated: true, Path: "/usr/bin/devin"},
		// integrated but NOT installed/chat-ready → excluded
		{Type: "qoder", Label: "Qoder", Installed: false, ChatReady: false, CcTransport: TransportCLIStream, Integrated: true},
		// installed but NOT integrated (detection-only) → excluded
		{Type: "openhands", Label: "OpenHands", Installed: true, ChatReady: true, CcTransport: TransportNone, Integrated: false, Path: "/usr/bin/openhands"},
	}
	c := fixtureCatalog(statuses)

	// "acp" is registered in cc-connect but has no catalog entry (it's the
	// generic, command-requiring type) — so it must never appear.
	registered := []string{"claudecode", "devin", "qoder", "acp", "tmux"}

	got := c.CreatableAgents(registered)

	byType := map[string]CreatableAgent{}
	for _, a := range got {
		byType[a.Type] = a
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 creatable agents, got %d: %+v", len(got), got)
	}
	if _, ok := byType["acp"]; ok {
		t.Error("generic acp must not be offered as creatable")
	}
	if _, ok := byType["qoder"]; ok {
		t.Error("uninstalled agent must be excluded")
	}
	if _, ok := byType["openhands"]; ok {
		t.Error("non-integrated agent must be excluded")
	}
	if cc := byType["claudecode"]; cc.Command != "" {
		t.Errorf("cli-stream agent should have no command, got %q", cc.Command)
	}
	if dv := byType["devin"]; dv.Command != "/usr/bin/devin" {
		t.Errorf("acp agent command = %q, want detected path", dv.Command)
	}
}

func TestCreatableAgentsExcludesUnregistered(t *testing.T) {
	// Installed + integrated, but not registered in cc-connect → excluded.
	c := fixtureCatalog([]AgentStatus{
		{Type: "cursor", Label: "Cursor", Installed: true, ChatReady: true, CcTransport: TransportCLIStream, Integrated: true, Path: "/usr/bin/agent"},
	})
	if got := c.CreatableAgents([]string{"claudecode"}); len(got) != 0 {
		t.Fatalf("expected no creatable agents when none registered, got %+v", got)
	}
}

func TestCommandForACPAgent(t *testing.T) {
	c := fixtureCatalog([]AgentStatus{
		{Type: "devin", CcTransport: TransportACP, Path: "/usr/bin/devin"},
		{Type: "claudecode", CcTransport: TransportCLIStream, Path: "/usr/bin/claude"},
	})
	if got := c.CommandForACPAgent("devin"); got != "/usr/bin/devin" {
		t.Errorf("CommandForACPAgent(devin) = %q, want /usr/bin/devin", got)
	}
	// CLI-stream agent is not ACP-driven → no command.
	if got := c.CommandForACPAgent("claudecode"); got != "" {
		t.Errorf("CommandForACPAgent(claudecode) = %q, want empty", got)
	}
	// Unknown / generic acp type has no catalog entry → no command.
	if got := c.CommandForACPAgent("acp"); got != "" {
		t.Errorf("CommandForACPAgent(acp) = %q, want empty", got)
	}
}
