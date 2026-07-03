package sourcecli

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// agentlyTool probes the 腾讯 Agent Mail CLI (`agently-cli`) that backs the
// agentmail data source. Two read-only commands:
//   - `agently-cli --version` → installed version
//   - `agently-cli +me`       → JSON {ok, data:{…}}: the authorized mailbox
//
// The CLI owns the mailbox credential (keychain), so — like lark-cli — logging in
// stays a manual user action; LoginHint carries the command the card shows with a
// copy button. There is no `update --check` command, so the update probe is
// skipped (UpdateAvailable stays false).
type agentlyTool struct {
	bin string
	run Runner
}

// NewAgentlyTool builds the agently-cli probe. bin defaults to "agently-cli";
// run defaults to shelling out (tests inject a fake).
func NewAgentlyTool(bin string, run Runner) *agentlyTool {
	if bin == "" {
		bin = "agently-cli"
	}
	if run == nil {
		run = defaultRunner
	}
	return &agentlyTool{bin: bin, run: run}
}

func (a *agentlyTool) Name() string { return "agently-cli" }

func (a *agentlyTool) Detect(ctx context.Context) CLIStatus {
	st := CLIStatus{
		Tool:        a.Name(),
		LoginHint:   a.bin + " auth login",
		InstallHint: "npm install -g @tencent-qqmail/agently-cli",
		CheckedAt:   time.Now().UTC(),
	}

	path, err := exec.LookPath(a.bin)
	if err != nil {
		return st // not installed — Installed stays false, hints guide the user
	}
	st.Installed = true
	st.Path = path

	if out, err := a.run(ctx, a.bin, "--version"); err == nil {
		st.Version = parseVersion(string(out))
	}

	// `+me` prints JSON on stdout for BOTH success and the expired-token case (the
	// latter also exits non-zero). Parse whatever body came back — it carries the
	// primary alias when authed, or a useful error message when not — and fall back
	// to the raw exec error only when there was no parseable body at all.
	out, err := a.run(ctx, a.bin, "+me")
	if len(out) > 0 {
		applyAgentlyMe(&st, out)
	}
	if !st.Authenticated && st.Error == "" && err != nil {
		st.Error = firstLine(err.Error())
	}

	return st
}

// agentlyMe mirrors `agently-cli +me`. Authorized:
//
//	{ok:true, data:{aliases:[{email,name,is_primary}], scopes:[…]}}
//
// Unauthorized / expired token:
//
//	{ok:false, error:{type,message}}   (also exits non-zero)
type agentlyMe struct {
	OK   bool `json:"ok"`
	Data struct {
		Aliases []struct {
			Email     string `json:"email"`
			Name      string `json:"name"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"aliases"`
		Scopes []string `json:"scopes"`
	} `json:"data"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func applyAgentlyMe(st *CLIStatus, out []byte) {
	var m agentlyMe
	if err := json.Unmarshal(out, &m); err != nil {
		st.Error = "+me: unparseable output"
		return
	}
	if !m.OK {
		st.Authenticated = false
		if m.Error.Message != "" {
			st.Error = m.Error.Message // e.g. "refresh token is invalid or expired…"
			st.TokenStatus = "expired"
		}
		return
	}
	st.Authenticated = true
	st.TokenStatus = "valid"
	st.Scopes = m.Data.Scopes
	// The mailbox address is the primary alias; fall back to the first listed.
	for _, a := range m.Data.Aliases {
		if st.AuthAccount == "" {
			st.AuthAccount = a.Email
		}
		if a.IsPrimary {
			st.AuthAccount = a.Email
			break
		}
	}
}
