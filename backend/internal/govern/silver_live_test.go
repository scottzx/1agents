package govern

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// TestSilverLive runs the real bronze (~/.1agents/sync.db) through the
// bronze→silver transform into a throwaway data.db and reports per-table counts
// plus a sample row. Guarded by RUN_SILVER_LIVE so `go test ./...` never touches
// real data. Run with:
//
//	RUN_SILVER_LIVE=1 go test ./internal/govern/ -run TestSilverLive -v
func TestSilverLive(t *testing.T) {
	if os.Getenv("RUN_SILVER_LIVE") == "" {
		t.Skip("set RUN_SILVER_LIVE=1 to run against real sync.db")
	}
	src, err := sources.OpenDefault() // real sync.db (read path)
	if err != nil {
		t.Fatalf("open real sync.db: %v", err)
	}
	dst, err := data.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open temp data.db: %v", err)
	}
	defer dst.Close()

	stats, err := Silver(src, dst)
	if err != nil {
		t.Fatalf("Silver: %v", err)
	}
	t.Logf("silver domain rows: contacts=%d messages=%d events=%d todos=%d",
		stats.Contacts, stats.Messages, stats.Events, stats.Todos)

	for _, tbl := range []string{
		"silver_icloud_contacts", "silver_feishu_users", "silver_feishu_messages",
		"silver_feishu_chats", "silver_microsoft_mail", "silver_agentmail_mail",
		"silver_microsoft_events", "silver_microsoft_todos",
	} {
		var n int
		dst.SQL().QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n)
		t.Logf("  %-26s total=%d", tbl, n)
	}

	// Lossless proof: 飞书 mentions extracted, contacts with a birthday preserved.
	var withMentions, withBday int
	dst.SQL().QueryRow(`SELECT COUNT(*) FROM silver_feishu_messages WHERE mentions != '[]'`).Scan(&withMentions)
	dst.SQL().QueryRow(`SELECT COUNT(*) FROM silver_icloud_contacts WHERE birthday != ''`).Scan(&withBday)
	t.Logf("  飞书 messages with @mentions=%d, icloud contacts with birthday=%d", withMentions, withBday)

	// Sample one 二级用户 discovered via a mention (has a real name).
	var openID, name, via string
	if err := dst.SQL().QueryRow(`SELECT external_id, name, discovered_via FROM silver_feishu_users
        WHERE name != '' LIMIT 1`).Scan(&openID, &name, &via); err == nil {
		t.Logf("  sample 二级用户: %s name=%q via=%s", openID, name, via)
	}
}
