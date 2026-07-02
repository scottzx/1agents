package sourcecli

import (
	"context"
	"os"
	"testing"
)

// TestLarkDetectLive probes the real installed lark-cli. Gated behind
// LARK_LIVE=1:
//
//	LARK_LIVE=1 go test ./internal/sourcecli/ -run TestLarkDetectLive -v
func TestLarkDetectLive(t *testing.T) {
	if os.Getenv("LARK_LIVE") != "1" {
		t.Skip("set LARK_LIVE=1 to run the live lark-cli probe")
	}
	st := NewLarkTool("", nil).Detect(context.Background())
	if !st.Installed {
		t.Fatalf("lark-cli not found on PATH")
	}
	t.Logf("installed=%v path=%s version=%s latest=%s updateAvail=%v",
		st.Installed, st.Path, st.Version, st.LatestVersion, st.UpdateAvailable)
	t.Logf("authenticated=%v tokenStatus=%s account=%s expiresAt=%v scopes=%d",
		st.Authenticated, st.TokenStatus, st.AuthAccount, st.AuthExpiresAt, len(st.Scopes))
	if st.Version == "" {
		t.Errorf("expected a version string")
	}
}
