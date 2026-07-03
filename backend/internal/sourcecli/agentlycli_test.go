package sourcecli

import (
	"context"
	"fmt"
	"testing"
)

func TestAgentlyDetectAuthenticated(t *testing.T) {
	// Real `agently-cli +me` schema (verified 2026-07-03 against v1.0.6): the
	// mailbox is the primary alias under data.aliases, plus granted scopes.
	responses := map[string]string{
		"--version": "agently-cli version 1.0.6",
		"+me": `{"ok":true,"data":{
			"aliases":[
				{"alias_id":"alias_x","email":"second@agent.qq.com","is_primary":false,"name":"second"},
				{"alias_id":"alias_y","email":"xiaofeng20260630@agent.qq.com","is_primary":true,"name":"xiaofeng20260630"}
			],
			"scopes":["alias:read","mail:read","mail:send","mail:delete"]
		}}`,
	}
	// bin="echo" exists on every box so Installed is true; the fake runner ignores bin.
	tool := NewAgentlyTool("echo", fakeRun(responses, nil))
	st := tool.Detect(context.Background())

	if !st.Installed {
		t.Fatal("expected installed")
	}
	if st.Version != "1.0.6" {
		t.Errorf("version = %q, want 1.0.6", st.Version)
	}
	if !st.Authenticated || st.TokenStatus != "valid" {
		t.Errorf("expected authenticated valid, got auth=%v status=%q", st.Authenticated, st.TokenStatus)
	}
	if st.AuthAccount != "xiaofeng20260630@agent.qq.com" {
		t.Errorf("account = %q, want the primary alias address", st.AuthAccount)
	}
	if len(st.Scopes) != 4 {
		t.Errorf("scopes = %v, want 4", st.Scopes)
	}
	if st.LoginHint == "" || st.InstallHint == "" {
		t.Error("expected login + install hints")
	}
}

// TestAgentlyMeExpired covers the expired-token body: +me exits non-zero but
// still prints {ok:false, error:{message}} — the probe surfaces that message.
func TestAgentlyMeExpired(t *testing.T) {
	var st CLIStatus
	applyAgentlyMe(&st, []byte(`{"ok":false,"error":{"type":"invalid_grant","message":"refresh token is invalid or expired, please re-authenticate"}}`))
	if st.Authenticated {
		t.Error("expired token must not be authenticated")
	}
	if st.TokenStatus != "expired" {
		t.Errorf("tokenStatus = %q, want expired", st.TokenStatus)
	}
	if st.Error == "" {
		t.Error("expected the error message surfaced")
	}
}

func TestAgentlyDetectNotLoggedIn(t *testing.T) {
	responses := map[string]string{"--version": "agently-cli v1.0.6"}
	errs := map[string]error{"+me": fmt.Errorf("not authorized\nrun `agently-cli auth login`")}
	tool := NewAgentlyTool("echo", fakeRun(responses, errs))
	st := tool.Detect(context.Background())

	if !st.Installed {
		t.Fatal("expected installed")
	}
	if st.Authenticated {
		t.Error("expected not authenticated")
	}
	if st.Error == "" {
		t.Error("expected the probe error surfaced")
	}
}

func TestAgentlyMeOKFalse(t *testing.T) {
	responses := map[string]string{
		"--version": "agently-cli v1.0.6",
		"+me":       `{"ok":false}`,
	}
	tool := NewAgentlyTool("echo", fakeRun(responses, nil))
	st := tool.Detect(context.Background())
	if st.Authenticated {
		t.Error("ok=false should not be authenticated")
	}
}
