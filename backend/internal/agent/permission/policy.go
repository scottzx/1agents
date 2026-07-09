// Package permission centralizes the "auto" permission-mode decision: given a
// tool request (its name, originating MCP server, and any MCP tool
// annotations), decide whether to auto-allow it or fall back to a user prompt.
//
// Why a package: issue #63 ("auto" permission mode) asks to collapse "which
// tools auto-run vs. which must ask" into ONE policy instead of scattering it
// across the three-way mode enum and ad-hoc hardcoding. The three legacy modes
// stay coarse:
//
//	approve-reads — auto-allow reads, prompt for everything else
//	approve-all   — auto-allow everything
//	deny-all      — auto-deny everything
//
// "auto" decides per request by SOURCE and RISK rather than the read/write
// split:
//
//	auto-allow — tools already context-locked by the backend (e.g. the
//	             project-locked mcp__project_items__* server), pure reads, and
//	             idempotent / reversible operations
//	prompt     — genuinely irreversible / externally-visible side effects:
//	             deletes, git push, chmod, sending messages, network writes,
//	             running arbitrary shell
//
// Decision basis, in priority order:
//  1. MCP tool annotations (readOnlyHint / destructiveHint / idempotentHint)
//  2. built-in allowlist / denylist, keyed by MCP server name or tool name
//
// The backend is a pass-through to the bridge-server (modules/1acp) for the
// live ACP permission callback; this package is the authoritative Go-side
// classifier the backend can consult and is the single source of truth for the
// "auto" rule table.
package permission

import "strings"

// Mode is the per-session permission policy. Mirrors the strings the
// bridge-server accepts and the frontend PermissionMode union.
type Mode string

const (
	ModeApproveReads Mode = "approve-reads"
	ModeApproveAll   Mode = "approve-all"
	ModeDenyAll      Mode = "deny-all"
	// ModeAuto decides per request via the source/risk policy table.
	ModeAuto Mode = "auto"
)

// IsValid reports whether m is one of the accepted permission modes. The empty
// string is NOT valid here (callers treat "" as "use the runtime default"
// separately).
func (m Mode) IsValid() bool {
	switch m {
	case ModeApproveReads, ModeApproveAll, ModeDenyAll, ModeAuto:
		return true
	default:
		return false
	}
}

// Decision is the outcome of evaluating a tool request under a mode.
type Decision int

const (
	// DecisionPrompt means defer to the user (show a permission bubble).
	DecisionPrompt Decision = iota
	// DecisionAllow means auto-approve without prompting.
	DecisionAllow
	// DecisionDeny means auto-reject without prompting.
	DecisionDeny
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	default:
		return "prompt"
	}
}

// Annotations carries the MCP tool annotation hints relevant to risk. All
// fields are pointers so "unset" is distinguishable from "false" — an absent
// hint must not be read as a safety guarantee.
type Annotations struct {
	// ReadOnlyHint: the tool does not modify its environment.
	ReadOnlyHint *bool
	// DestructiveHint: the tool may perform irreversible updates (only
	// meaningful when ReadOnlyHint is false/unset).
	DestructiveHint *bool
	// IdempotentHint: repeated calls with the same args have no additional
	// effect (only meaningful when ReadOnlyHint is false/unset).
	IdempotentHint *bool
}

// Request describes one tool invocation the agent wants to make.
type Request struct {
	// ToolName is the fully-qualified tool name, e.g. "Read", "Bash",
	// "mcp__project_items__create_project_item".
	ToolName string
	// ServerName is the MCP server the tool belongs to, when known (e.g.
	// "project_items"). Derived from ToolName when empty.
	ServerName string
	// Annotations are the MCP tool annotations, when the agent advertised them.
	Annotations Annotations
}

// server returns the effective MCP server name: the explicit ServerName, else
// the segment parsed from an "mcp__<server>__<tool>" tool name, else "".
func (r Request) server() string {
	if r.ServerName != "" {
		return r.ServerName
	}
	return mcpServerOf(r.ToolName)
}

// mcpServerOf extracts "<server>" from "mcp__<server>__<tool>". Returns "" for
// non-MCP (built-in) tools.
func mcpServerOf(toolName string) string {
	const prefix = "mcp__"
	if !strings.HasPrefix(toolName, prefix) {
		return ""
	}
	rest := toolName[len(prefix):]
	if i := strings.Index(rest, "__"); i >= 0 {
		return rest[:i]
	}
	return rest
}

// contextLockedServers are MCP servers the backend hard-locks to the current
// project/task via env injection, so the agent cannot escalate scope through
// them. Their tools are safe to auto-allow under "auto" — this is exactly the
// PM project-items-tool case from issue #63 that approve-reads wrongly prompted on.
// "tasks" is kept as a legacy alias for the pre-rename server name.
var contextLockedServers = map[string]bool{
	"project_items": true,
	"tasks":         true,
}

// readOnlyTools are built-in tools known to be pure reads (no annotations
// available for built-ins, so we classify by name).
var readOnlyTools = map[string]bool{
	"Read":         true,
	"Glob":         true,
	"Grep":         true,
	"LS":           true,
	"NotebookRead": true,
	"TodoRead":     true,
	"WebFetch":     true,
	"WebSearch":    true,
}

// highRiskTools are built-in tools with irreversible or externally-visible
// effects. These always prompt under "auto" even if an (untrusted) annotation
// claims otherwise.
var highRiskTools = map[string]bool{
	"Bash":         true, // arbitrary shell
	"BashOutput":   true,
	"KillBash":     true,
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

// Evaluate decides what to do with req under mode. Built-in tool names are
// matched case-sensitively (they use fixed casing); the MCP server name is
// parsed from the tool name when not given explicitly.
func Evaluate(mode Mode, req Request) Decision {
	switch mode {
	case ModeApproveAll:
		return DecisionAllow
	case ModeDenyAll:
		return DecisionDeny
	case ModeApproveReads:
		if isReadOnly(req) {
			return DecisionAllow
		}
		return DecisionPrompt
	case ModeAuto:
		return evaluateAuto(req)
	default:
		// Unknown/empty mode: be conservative, prompt.
		return DecisionPrompt
	}
}

// evaluateAuto applies the source/risk policy table for ModeAuto.
//
// Order matters:
//  1. context-locked MCP server    -> allow (source-based, highest trust)
//  2. high-risk built-in tool      -> prompt (risk-based, overrides weak hints)
//  3. read-only (built-in or hint) -> allow
//  4. destructive hint             -> prompt
//  5. idempotent hint (reversible) -> allow
//  6. default                      -> prompt (unknown side effects)
func evaluateAuto(req Request) Decision {
	if contextLockedServers[req.server()] {
		return DecisionAllow
	}
	if highRiskTools[req.ToolName] {
		return DecisionPrompt
	}
	if isReadOnly(req) {
		return DecisionAllow
	}
	if isDestructive(req) {
		return DecisionPrompt
	}
	if isIdempotent(req) {
		return DecisionAllow
	}
	return DecisionPrompt
}

// isReadOnly reports whether req is a pure read, by built-in name or by an
// explicit readOnlyHint=true annotation.
func isReadOnly(req Request) bool {
	if readOnlyTools[req.ToolName] {
		return true
	}
	if h := req.Annotations.ReadOnlyHint; h != nil && *h {
		return true
	}
	return false
}

// isDestructive reports an explicit destructiveHint=true.
func isDestructive(req Request) bool {
	h := req.Annotations.DestructiveHint
	return h != nil && *h
}

// isIdempotent reports an explicit idempotentHint=true.
func isIdempotent(req Request) bool {
	h := req.Annotations.IdempotentHint
	return h != nil && *h
}
