package govern

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sources"

	_ "modernc.org/sqlite"
)

func openStore(t *testing.T) *sources.Store {
	t.Helper()
	dsn := "file:" + url.PathEscape(filepath.Join(t.TempDir(), "sync.db")) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sync.db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	st, err := sources.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return st
}

func openContacts(t *testing.T) *meta.ContactStore {
	t.Helper()
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open meta.db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return meta.NewContactStore(db)
}

func vcard(uid, fn, tel string) string {
	return "BEGIN:VCARD\nVERSION:3.0\nUID:" + uid + "\nFN:" + fn + "\nTEL:" + tel + "\nEND:VCARD"
}

// TestICloudContactsGovern proves bronze→gold shaping, the governance window
// (only records changed since the last run are processed → an unchanged govern
// is a no-op), and the fetch/govern decoupling (a reset cursor re-shapes gold
// from bronze with no network).
func TestICloudContactsGovern(t *testing.T) {
	st := openStore(t)
	cs := openContacts(t)
	seed := func(uid, fn, tel, etag string) {
		t.Helper()
		if _, err := st.CommitPage(sources.SourceICloud, "default", []sources.RawRecord{
			{Kind: sources.KindContact, Collection: "/b/", UID: "/b/" + uid, ETag: etag,
				ContentType: "text/vcard", Payload: vcard(uid, fn, tel)},
		}, sources.Cursor{Kind: "sync_token", Value: "t"}); err != nil {
			t.Fatalf("seed bronze %s: %v", uid, err)
		}
	}

	seed("a.vcf", "Alice", "13800000001", `"1"`)
	seed("b.vcf", "Bob", "13800000002", `"1"`)

	// First govern → both contacts created.
	created, updated, err := ICloudContacts(st, cs)
	if err != nil || created != 2 || updated != 0 {
		t.Fatalf("govern 1: created=%d updated=%d err=%v", created, updated, err)
	}
	if n := count(t, cs); n != 2 {
		t.Fatalf("gold: %d contacts, want 2", n)
	}

	// Re-govern with no new bronze → no-op (the window is empty).
	if c, u, err := ICloudContacts(st, cs); err != nil || c != 0 || u != 0 {
		t.Fatalf("govern 2 (no-op): created=%d updated=%d err=%v", c, u, err)
	}

	// A changed vCard (new etag bumps fetched_at) → only that one is re-governed.
	time.Sleep(2 * time.Millisecond)
	seed("a.vcf", "Alice Smith", "13800000001", `"2"`)
	if c, u, err := ICloudContacts(st, cs); err != nil || c != 0 || u != 1 {
		t.Fatalf("govern 3 (incremental): created=%d updated=%d err=%v", c, u, err)
	}
	if name(t, cs, "13800000001") != "Alice Smith" {
		t.Fatalf("gold not updated to Alice Smith")
	}

	// Decoupling: reset the govern cursor and re-run — gold is rebuilt from bronze
	// offline (no puller, no network), proving govern never re-fetches.
	if err := st.SaveGovernCursor(sources.SourceICloud, sources.KindContact, 0); err != nil {
		t.Fatalf("reset cursor: %v", err)
	}
	if c, u, err := ICloudContacts(st, cs); err != nil || c != 0 || u != 2 {
		t.Fatalf("re-govern from reset: created=%d updated=%d err=%v", c, u, err)
	}
}

func count(t *testing.T, cs *meta.ContactStore) int {
	t.Helper()
	list, err := cs.ListContacts()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return len(list)
}

func name(t *testing.T, cs *meta.ContactStore, phone string) string {
	t.Helper()
	list, err := cs.ListContacts()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, c := range list {
		if c.Phone == phone {
			return c.Name
		}
	}
	return ""
}
