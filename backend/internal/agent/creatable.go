package agent

// CreatableAgent is a cc-connect agent type this host can actually create a
// working project for: it is integrated (this backend knows how to drive it),
// available (its CLI is installed or its ACP adapter is vendored), and
// registered in cc-connect. The generic "acp" type and detection-only
// frameworks are deliberately excluded — they either need extra required
// config this host can't supply (acp → "command") or this backend can't drive
// them yet. This is the single source of truth that keeps un-creatable agents
// (which would brick cc-connect on startup) out of the project-create UI.
type CreatableAgent struct {
	Type        AgentType   `json:"type"`
	Label       string      `json:"label"`
	CcTransport CcTransport `json:"cc_transport"`
	// Command is the detected binary path for ACP-driven agents
	// (CcTransport == "acp"); used to auto-fill the required "command" option
	// at create time. Empty for CLI-stream agents, which need no command.
	Command string `json:"command,omitempty"`
}

// CreatableAgents intersects this host's install/ACP catalog with the set of
// agent names cc-connect has actually registered, returning only the agents
// that are safe to create with just a work_dir (plus an auto-derived command
// for ACP transport). registered is core.ListRegisteredAgents() from
// cc-connect.
func (c *CatalogStore) CreatableAgents(registered []string) []CreatableAgent {
	reg := make(map[string]bool, len(registered))
	for _, r := range registered {
		reg[r] = true
	}
	out := make([]CreatableAgent, 0, len(reg))
	for _, st := range c.Snapshot() {
		if !st.Integrated || !(st.Installed || st.ChatReady) {
			continue
		}
		if !reg[string(st.Type)] {
			continue
		}
		ca := CreatableAgent{Type: st.Type, Label: st.Label, CcTransport: st.CcTransport}
		if st.CcTransport == TransportACP {
			ca.Command = st.Path
		}
		out = append(out, ca)
	}
	return out
}

// CommandForACPAgent returns the detected binary path to drive agentType over
// ACP, or "" if agentType is not an installed ACP-transport agent in the
// catalog. Used to auto-fill the required "command" option when a project is
// created, so an ACP agent never bricks on a missing path.
func (c *CatalogStore) CommandForACPAgent(agentType string) string {
	for _, st := range c.Snapshot() {
		if string(st.Type) == agentType && st.CcTransport == TransportACP {
			return st.Path
		}
	}
	return ""
}
