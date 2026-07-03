package sourcecli

import (
	"context"
	"fmt"
	"testing"
)

func TestAgentlyDetectAuthenticated(t *testing.T) {
	responses := map[string]string{
		"--version": "agently-cli v1.0.6",
		"+me":       `{"ok":true,"data":{"email":"xiaofeng20260630@agent.qq.com","name":"晓峰"}}`,
	}
	// bin="echo" exists on every box so Installed is true; the fake runner ignores bin.
	tool := NewAgentlyTool("echo", fakeRun(responses, nil))
	st := tool.Detect(context.Background())

	if !st.Installed {
		t.Fatal("expected installed")
	}
	if st.Version != "v1.0.6" {
		t.Errorf("version = %q, want v1.0.6", st.Version)
	}
	if !st.Authenticated || st.TokenStatus != "valid" {
		t.Errorf("expected authenticated valid, got auth=%v status=%q", st.Authenticated, st.TokenStatus)
	}
	if st.AuthAccount != "xiaofeng20260630@agent.qq.com" {
		t.Errorf("account = %q, want the mailbox address", st.AuthAccount)
	}
	if st.LoginHint == "" || st.InstallHint == "" {
		t.Error("expected login + install hints")
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
