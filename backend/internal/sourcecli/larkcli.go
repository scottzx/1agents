package sourcecli

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// larkTool probes the Feishu/Lark CLI (`lark-cli`). It reads three commands, all
// read-only:
//   - `lark-cli --version`            → installed version
//   - `lark-cli auth status`          → JSON: tokenStatus/expiresAt/userName/scope
//   - `lark-cli update --check --json` → JSON: current/latest_version, action
//
// Login and update are left to the user; LoginHint/UpdateHint carry the commands
// the card shows with a copy button.
type larkTool struct {
	bin string
	run Runner
}

// NewLarkTool builds the lark-cli probe. bin defaults to "lark-cli"; run
// defaults to shelling out (tests inject a fake).
func NewLarkTool(bin string, run Runner) *larkTool {
	if bin == "" {
		bin = "lark-cli"
	}
	if run == nil {
		run = defaultRunner
	}
	return &larkTool{bin: bin, run: run}
}

func (l *larkTool) Name() string { return "lark-cli" }

// updateCheckTimeout bounds the network-bound `update --check` so a slow or
// offline network can't hang the whole status probe.
const updateCheckTimeout = 8 * time.Second

func (l *larkTool) Detect(ctx context.Context) CLIStatus {
	st := CLIStatus{
		Tool:        l.Name(),
		LoginHint:   l.bin + " auth login",
		UpdateHint:  l.bin + " update",
		InstallHint: "npm install -g @larksuite/cli",
		CheckedAt:   time.Now().UTC(),
	}

	path, err := exec.LookPath(l.bin)
	if err != nil {
		return st // not installed — Installed stays false, hints guide the user
	}
	st.Installed = true
	st.Path = path

	// Version — cheap, offline.
	if out, err := l.run(ctx, l.bin, "--version"); err == nil {
		st.Version = parseVersion(string(out))
	}

	// Auth — `auth status` prints JSON to stdout even on a stale token; a hard
	// error (no config / never logged in) means unauthenticated.
	if out, err := l.run(ctx, l.bin, "auth", "status"); err == nil {
		applyAuthStatus(&st, out)
	} else {
		st.Authenticated = false
		st.Error = firstLine(err.Error())
	}

	// Update — network-bound; give it its own timeout and treat failure as
	// "unknown", not fatal.
	uctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	if out, err := l.run(uctx, l.bin, "update", "--check", "--json"); err == nil {
		applyUpdateCheck(&st, out)
	}

	return st
}

// larkAuthStatus mirrors the fields of `lark-cli auth status` we consume.
type larkAuthStatus struct {
	Identity         string `json:"identity"`
	TokenStatus      string `json:"tokenStatus"`
	UserName         string `json:"userName"`
	ExpiresAt        string `json:"expiresAt"`
	RefreshExpiresAt string `json:"refreshExpiresAt"`
	Scope            string `json:"scope"`
}

func applyAuthStatus(st *CLIStatus, out []byte) {
	var a larkAuthStatus
	if err := json.Unmarshal(out, &a); err != nil {
		st.Error = "auth status: unparseable output"
		return
	}
	st.TokenStatus = a.TokenStatus
	st.Authenticated = a.TokenStatus == "valid"
	st.AuthAccount = a.UserName
	st.AuthIdentity = a.Identity
	st.AuthExpiresAt = parseTime(a.ExpiresAt)
	st.RefreshExpiresAt = parseTime(a.RefreshExpiresAt)
	if a.Scope != "" {
		st.Scopes = strings.Fields(a.Scope)
	}
}

// larkUpdateCheck mirrors `lark-cli update --check --json`.
type larkUpdateCheck struct {
	Action         string `json:"action"` // "update_available" | "up_to_date" | ...
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
}

func applyUpdateCheck(st *CLIStatus, out []byte) {
	var u larkUpdateCheck
	if err := json.Unmarshal(out, &u); err != nil {
		return
	}
	st.LatestVersion = u.LatestVersion
	if st.Version == "" && u.CurrentVersion != "" {
		st.Version = u.CurrentVersion
	}
	st.UpdateAvailable = u.Action == "update_available"
}

// parseVersion pulls the semver out of `lark-cli version 1.0.18`.
func parseVersion(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// parseTime parses an RFC3339 timestamp, returning nil on empty/invalid input.
func parseTime(s string) *time.Time {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
