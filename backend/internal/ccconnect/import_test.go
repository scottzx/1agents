package ccconnect

import (
	"testing"

	"github.com/chenhg5/cc-connect/config"
)

func TestChannelIdentity(t *testing.T) {
	// Two Feishu bots with different app_id must get DIFFERENT identities so
	// incremental import keeps both (the "渠道 ID" match key).
	a := config.PlatformConfig{Type: "feishu", Options: map[string]any{"app_id": "cli_A", "app_secret": "s1"}}
	b := config.PlatformConfig{Type: "feishu", Options: map[string]any{"app_id": "cli_B", "app_secret": "s2"}}
	if channelIdentity(a) == channelIdentity(b) {
		t.Fatalf("two feishu bots collapsed to same identity: %q", channelIdentity(a))
	}
	// Same app_id (even if other opts differ) → same channel → dedup on re-import.
	a2 := config.PlatformConfig{Type: "feishu", Options: map[string]any{"app_id": "cli_A", "app_secret": "changed"}}
	if channelIdentity(a) != channelIdentity(a2) {
		t.Fatalf("same app_id produced different identities: %q vs %q", channelIdentity(a), channelIdentity(a2))
	}
}

func TestUniqueProjectID(t *testing.T) {
	inUse := map[string]bool{"demo": true, "demo-1": true}
	if got := uniqueProjectID(inUse, "demo"); got != "demo-2" {
		t.Errorf("uniqueProjectID = %q, want demo-2", got)
	}
	if got := uniqueProjectID(inUse, "fresh"); got != "fresh" {
		t.Errorf("uniqueProjectID = %q, want fresh", got)
	}
	if got := uniqueProjectID(inUse, ""); got != "ws" {
		t.Errorf("uniqueProjectID(empty) = %q, want ws", got)
	}
}
