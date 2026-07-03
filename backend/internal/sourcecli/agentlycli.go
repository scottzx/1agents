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

	// `+me` prints the authorized mailbox as JSON on success; a hard error means
	// the CLI is installed but not logged in.
	if out, err := a.run(ctx, a.bin, "+me"); err == nil {
		applyAgentlyMe(&st, out)
	} else {
		st.Authenticated = false
		st.Error = firstLine(err.Error())
	}

	return st
}

// agentlyMe mirrors `agently-cli +me`: {ok, data:{…}}. The mailbox address field
// name is read loosely (email/address/mail/account) so a minor schema change
// still surfaces the account.
type agentlyMe struct {
	OK   bool                       `json:"ok"`
	Data map[string]json.RawMessage `json:"data"`
}

func applyAgentlyMe(st *CLIStatus, out []byte) {
	var m agentlyMe
	if err := json.Unmarshal(out, &m); err != nil {
		st.Error = "+me: unparseable output"
		return
	}
	if !m.OK {
		st.Authenticated = false
		return
	}
	st.Authenticated = true
	st.TokenStatus = "valid"
	for _, k := range []string{"email", "address", "mail", "account", "name"} {
		if raw, ok := m.Data[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				st.AuthAccount = s
				break
			}
		}
	}
}
