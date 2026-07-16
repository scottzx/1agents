package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// CcTransport is how this system actually drives an agent today.
type CcTransport = string

const (
	// TransportACP drives the agent over the standard Agent Client Protocol
	// (JSON-RPC over stdio). Only Devin uses this in cc-connect today.
	TransportACP CcTransport = "acp"
	// TransportCLIStream drives the agent over its private CLI stream
	// (stream-json / --format json / PTY transcript parsing).
	TransportCLIStream CcTransport = "cli-stream"
	// TransportNone means cc-connect cannot drive this agent yet
	// (detection-only entry); empty so the frontend hides the badge.
	TransportNone CcTransport = ""
)

// AgentDescriptor is the static, hand-maintained capability entry for one
// agent application. It records the upstream app's own capabilities
// (AcpCapable / CliCapable), how cc-connect currently drives it (CcTransport),
// whether this backend can drive it at all (Integrated), and the terminal
// command to install it (InstallCommand).
type AgentDescriptor struct {
	Type        AgentType
	Label       string
	Binary      string // binary probed via exec.LookPath
	AcpCapable  bool
	CliCapable  bool
	CcTransport CcTransport
	// Integrated is true when this backend can actually drive the agent through
	// either cc-connect or the 1acp web-chat bridge. Only integrated agents are
	// offered in the chat agent picker; the rest are detection-only.
	Integrated bool
	// InstallCommand is the terminal command a user runs to install the agent.
	// Surfaced as a copyable button when the agent isn't installed.
	InstallCommand string
	// AdapterPackage is the npm package of the ACP adapter the web chat path
	// (1acp bridge) launches for this agent, when it differs from the CLI
	// Binary probed above. The adapter is resolved from modules/1acp's
	// node_modules, so an agent can be chat-ready without its own CLI on
	// PATH. Empty when the agent has no dedicated vendored ACP adapter.
	AdapterPackage string
}

// AgentCatalog is the canonical capability table.
//
// The first block is the agents the backend can drive through cc-connect or
// the 1acp web-chat bridge (Integrated: true). The second block is trendy agent
// frameworks we detect and offer install guidance for, but don't yet drive
// (Integrated: false) — they appear in the settings detection list only, never
// in the chat picker.
//
// Binary names mirror cc-connect's agent implementations. Per the official ACP
// registry (cdn.agentclientprotocol.com) the integrated coding agents all ship
// both an ACP mode and a CLI mode.
var AgentCatalog = []AgentDescriptor{
	// ── Integrated: drivable by this backend ────────────────────────────────
	{Type: AgentTypeClaudecode, Label: "Claude Code", Binary: "claude", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "npm install -g @anthropic-ai/claude-code", AdapterPackage: "@agentclientprotocol/claude-agent-acp"},
	// codex also has an app_server (WebSocket RPC) mode, but cc-connect's
	// default "exec" backend drives it as a CLI stream.
	{Type: AgentTypeCodex, Label: "Codex", Binary: "codex", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "npm install -g @openai/codex", AdapterPackage: "@agentclientprotocol/codex-acp"},
	// Grok Build exposes ACP natively through `grok agent stdio`; unlike the
	// adapter-backed entries above, it is chat-ready only when the Grok CLI is
	// actually installed on PATH.
	{Type: AgentTypeGrokBuild, Label: "Grok", Binary: "grok", AcpCapable: true, CliCapable: true, CcTransport: TransportACP, Integrated: true, InstallCommand: "curl -fsSL https://x.ai/cli/install.sh | bash"},
	{Type: AgentTypeCursor, Label: "Cursor Agent", Binary: "agent", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "curl https://cursor.com/install -fsS | bash"},
	{Type: AgentTypeGemini, Label: "Gemini", Binary: "gemini", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "npm install -g @google/gemini-cli"},
	{Type: AgentTypeDevin, Label: "Devin", Binary: "devin", AcpCapable: true, CliCapable: true, CcTransport: TransportACP, Integrated: true, InstallCommand: "curl -fsSL https://cli.devin.ai/install.sh | bash"},
	{Type: AgentTypeIflow, Label: "iFlow", Binary: "iflow", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "npm install -g @iflow-ai/iflow-cli"},
	{Type: AgentTypeKimi, Label: "Kimi", Binary: "kimi", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "uv tool install --python 3.13 kimi-cli"},
	{Type: AgentTypeOpencode, Label: "OpenCode", Binary: "opencode", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "curl -fsSL https://opencode.ai/install | bash", AdapterPackage: "opencode-ai"},
	{Type: AgentTypePi, Label: "Pi", Binary: "pi", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "npm install -g @mariozechner/pi-coding-agent"},
	{Type: AgentTypeQoder, Label: "Qoder", Binary: "qodercli", AcpCapable: true, CliCapable: true, CcTransport: TransportCLIStream, Integrated: true, InstallCommand: "npm install -g @qoder-ai/qodercli"},

	// ── Detection-only: known frameworks we can detect + guide install for, ──
	//    but this backend can't drive yet (not in runner.go imports).
	{Type: "antigravity", Label: "Antigravity", Binary: "agy", AcpCapable: false, CliCapable: true, CcTransport: TransportNone, Integrated: false, InstallCommand: "curl -fsSL https://antigravity.google/cli/install.sh | bash"},
	{Type: "openhands", Label: "OpenHands", Binary: "openhands", AcpCapable: true, CliCapable: true, CcTransport: TransportNone, Integrated: false, InstallCommand: "uvx --python 3.12 --from openhands-ai openhands"},
	{Type: "trae", Label: "Trae", Binary: "trae-cli", AcpCapable: false, CliCapable: true, CcTransport: TransportNone, Integrated: false, InstallCommand: "git clone https://github.com/bytedance/trae-agent && cd trae-agent && uv sync --all-extras"},
	{Type: "openclaw", Label: "OpenClaw", Binary: "openclaw", AcpCapable: false, CliCapable: true, CcTransport: TransportNone, Integrated: false, InstallCommand: "npm install -g openclaw@latest"},
	{Type: "hermes", Label: "Hermes", Binary: "hermes", AcpCapable: true, CliCapable: true, CcTransport: TransportNone, Integrated: false, InstallCommand: "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash"},
}

// AgentStatus is one AgentDescriptor enriched with the per-host install probe.
type AgentStatus struct {
	Type           AgentType   `json:"type"`
	Label          string      `json:"label"`
	Binary         string      `json:"binary"`
	Installed      bool        `json:"installed"`
	Path           string      `json:"path,omitempty"`
	AcpCapable     bool        `json:"acp_capable"`
	CliCapable     bool        `json:"cli_capable"`
	CcTransport    CcTransport `json:"cc_transport"`
	Integrated     bool        `json:"integrated"`
	InstallCommand string      `json:"install_command,omitempty"`
	// ChatReady is true when the web chat path can launch this agent: either
	// its CLI binary is installed, or its ACP adapter is vendored in
	// modules/1acp/node_modules. The chat picker gates on this; the settings
	// detection list keeps using Installed (the CLI probe).
	ChatReady bool `json:"chat_ready"`
}

// CatalogStore holds the globally-detected agent install state. It probes the
// system PATH once at startup and caches the result behind an RWMutex; callers
// can force a re-probe via Scan (wired to the ?refresh=1 endpoint param).
type CatalogStore struct {
	mu        sync.RWMutex
	statuses  []AgentStatus
	scannedAt time.Time
}

// NewCatalogStore constructs the store and performs the initial probe.
func NewCatalogStore() *CatalogStore {
	c := &CatalogStore{}
	c.Scan()
	return c
}

var (
	defaultCatalog     *CatalogStore
	defaultCatalogOnce sync.Once
)

// DefaultCatalog returns the process-wide catalog store, constructing it on
// first use. Both the HTTP catalog endpoint and the cc-connect runner share
// this single instance so a manual ?refresh=1 stays consistent across them and
// neither depends on startup ordering.
func DefaultCatalog() *CatalogStore {
	defaultCatalogOnce.Do(func() { defaultCatalog = NewCatalogStore() })
	return defaultCatalog
}

// Scan re-probes every descriptor's binary via exec.LookPath (instant; no
// --version exec) and atomically replaces the cached snapshot.
func (c *CatalogStore) Scan() []AgentStatus {
	adapterDir := acpAdapterDir()
	statuses := make([]AgentStatus, 0, len(AgentCatalog))
	for _, d := range AgentCatalog {
		st := AgentStatus{
			Type:           d.Type,
			Label:          d.Label,
			Binary:         d.Binary,
			AcpCapable:     d.AcpCapable,
			CliCapable:     d.CliCapable,
			CcTransport:    d.CcTransport,
			Integrated:     d.Integrated,
			InstallCommand: d.InstallCommand,
		}
		if path, err := exec.LookPath(d.Binary); err == nil {
			st.Installed = true
			st.Path = path
		}
		// Web chat launches the ACP adapter (from modules/1acp), not the CLI,
		// so an agent is chat-ready when either the CLI is on PATH or its
		// adapter package is vendored.
		st.ChatReady = st.Installed || adapterVendored(adapterDir, d.AdapterPackage)
		statuses = append(statuses, st)
	}

	c.mu.Lock()
	c.statuses = statuses
	c.scannedAt = time.Now()
	c.mu.Unlock()

	return statuses
}

// acpAdapterDir resolves modules/1acp/node_modules by walking up from the
// working directory (mirrors the lookup in internal/supervisor/acpx.go).
// Returns "" if modules/1acp can't be located.
func acpAdapterDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, "modules", "1acp")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Join(candidate, "node_modules")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// adapterVendored reports whether the named ACP adapter package is installed
// under modules/1acp/node_modules (so the web chat path can launch it without
// a runtime npx download). A blank pkg or unresolved adapter dir returns false.
func adapterVendored(adapterDir, pkg string) bool {
	if adapterDir == "" || pkg == "" {
		return false
	}
	manifest := filepath.Join(adapterDir, filepath.FromSlash(pkg), "package.json")
	_, err := os.Stat(manifest)
	return err == nil
}

// Snapshot returns a copy of the cached statuses under a read lock.
func (c *CatalogStore) Snapshot() []AgentStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]AgentStatus, len(c.statuses))
	copy(out, c.statuses)
	return out
}
