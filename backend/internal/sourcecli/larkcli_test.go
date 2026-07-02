package sourcecli

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// fakeRun dispatches canned output by the first arg (subcommand).
func fakeRun(responses map[string]string, errs map[string]error) Runner {
	return func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		key := ""
		if len(args) > 0 {
			key = args[0]
		}
		if err, ok := errs[key]; ok {
			return nil, err
		}
		return []byte(responses[key]), nil
	}
}

func TestLarkDetectAuthenticated(t *testing.T) {
	responses := map[string]string{
		"--version": "lark-cli version 1.0.18",
		"auth": `{
			"identity": "user",
			"tokenStatus": "valid",
			"userName": "曾晓峰",
			"expiresAt": "2026-07-02T01:15:11+08:00",
			"refreshExpiresAt": "2026-07-08T23:15:11+08:00",
			"scope": "im:message contact:user:search calendar:calendar:read"
		}`,
		"update": `{"action":"update_available","current_version":"1.0.18","latest_version":"1.0.63"}`,
	}
	// Use the real LookPath-independent path by pointing bin at a command that
	// exists on every box ("echo") so Installed is true; the fake runner ignores bin.
	tool := NewLarkTool("echo", fakeRun(responses, nil))
	st := tool.Detect(context.Background())

	if !st.Installed {
		t.Fatalf("expected installed")
	}
	if st.Version != "1.0.18" {
		t.Errorf("version = %q, want 1.0.18", st.Version)
	}
	if !st.Authenticated || st.TokenStatus != "valid" {
		t.Errorf("expected authenticated valid, got auth=%v status=%q", st.Authenticated, st.TokenStatus)
	}
	if st.AuthAccount != "曾晓峰" {
		t.Errorf("account = %q", st.AuthAccount)
	}
	if st.AuthExpiresAt == nil || st.AuthExpiresAt.Year() != 2026 {
		t.Errorf("expiresAt not parsed: %v", st.AuthExpiresAt)
	}
	if len(st.Scopes) != 3 {
		t.Errorf("scopes = %v, want 3", st.Scopes)
	}
	if !st.UpdateAvailable || st.LatestVersion != "1.0.63" {
		t.Errorf("update: avail=%v latest=%q", st.UpdateAvailable, st.LatestVersion)
	}
	if st.LoginHint == "" {
		t.Errorf("expected login hint")
	}
}

func TestLarkDetectNotLoggedIn(t *testing.T) {
	responses := map[string]string{"--version": "lark-cli version 1.0.18"}
	errs := map[string]error{"auth": fmt.Errorf("no token\nrun `lark-cli auth login`")}
	tool := NewLarkTool("echo", fakeRun(responses, errs))
	st := tool.Detect(context.Background())

	if st.Authenticated {
		t.Errorf("expected not authenticated")
	}
	if st.Error != "no token" {
		t.Errorf("error = %q, want first line only", st.Error)
	}
}

func TestManagerCaches(t *testing.T) {
	var calls int
	tool := &countingTool{onDetect: func() { calls++ }}
	m := NewManager(time.Hour)
	m.Register(tool)

	if _, ok := m.Status(context.Background(), "counting"); !ok {
		t.Fatal("tool not found")
	}
	m.Status(context.Background(), "counting") // cached
	if calls != 1 {
		t.Errorf("Detect called %d times, want 1 (cached)", calls)
	}
	m.Recheck(context.Background(), "counting")
	if calls != 2 {
		t.Errorf("after recheck Detect called %d times, want 2", calls)
	}
}

type countingTool struct{ onDetect func() }

func (c *countingTool) Name() string { return "counting" }
func (c *countingTool) Detect(ctx context.Context) CLIStatus {
	c.onDetect()
	return CLIStatus{Tool: "counting"}
}
