package govern

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// TestGoldLive runs the real bronze (~/.1agents/sync.db) through the full
// bronze→silver→gold pipeline into a throwaway data.db and reports gold counts
// plus a sampled @mention that resolved to a contact — the issue #400 acceptance
// check. Guarded by RUN_SILVER_LIVE so `go test ./...` never touches real data:
//
//	RUN_SILVER_LIVE=1 go test ./internal/govern/ -run TestGoldLive -v
func TestGoldLive(t *testing.T) {
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

	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver: %v", err)
	}
	st, err := Gold(dst)
	if err != nil {
		t.Fatalf("Gold: %v", err)
	}
	t.Logf("gold fusion: threads=%d messages=%d new-contacts=%d", st.Threads, st.Messages, st.Contacts)

	db := dst.SQL()
	for _, tbl := range []string{"contacts", "contact_channels", "threads", "messages", "message_participants"} {
		var n int
		db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n)
		t.Logf("  %-22s total=%d", tbl, n)
	}

	// Every 二级用户 with a name should have a feishu channel + contact.
	var namedUsers, resolvedChannels int
	db.QueryRow(`SELECT COUNT(*) FROM silver_feishu_users WHERE name != ''`).Scan(&namedUsers)
	db.QueryRow(`SELECT COUNT(*) FROM contact_channels WHERE platform='feishu' AND contact_id != ''`).Scan(&resolvedChannels)
	t.Logf("  named 二级用户=%d, resolved feishu channels=%d", namedUsers, resolvedChannels)

	// Sample: a message whose @mention resolved to a named contact (the acceptance
	// case — "this @ is who").
	var msgID, mentionName, mentionOpen string
	err = db.QueryRow(`SELECT m.external_id, c.name, ch.address
        FROM message_participants mp
        JOIN contacts c         ON c.id = mp.contact_id
        JOIN contact_channels ch ON ch.contact_id = c.id AND ch.platform='feishu'
        JOIN messages m         ON m.id = mp.message_id
        WHERE mp.role='mention' AND c.name != '' LIMIT 1`).Scan(&msgID, &mentionName, &mentionOpen)
	if err != nil {
		t.Logf("  (no resolved @mention sample: %v)", err)
	} else {
		t.Logf("  sample @mention: message %s @%s (%s)", msgID, mentionName, mentionOpen)
	}
}
