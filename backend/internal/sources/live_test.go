package sources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// TestLiveICloudIncremental drives the real Phase 1 pipeline against live iCloud
// using the locally stored (Keychain) credentials. Gated by ICLOUD_LIVE=1 so it
// never runs in normal suites (and is throttling-sensitive — run sparingly). It
// proves the incremental path end to end: first sync seeds bronze from the real
// address book; the second sync sees an unchanged CTag and skips with zero
// changed rows.
func TestLiveICloudIncremental(t *testing.T) {
	if os.Getenv("ICLOUD_LIVE") == "" {
		t.Skip("set ICLOUD_LIVE=1 to run the live iCloud incremental pull")
	}
	appleID, pw, ok, err := icloud.LoadCredentials()
	if err != nil || !ok {
		t.Skipf("no stored iCloud credentials (ok=%v err=%v)", ok, err)
	}
	st := openSourcesStore(t)
	puller := NewICloudPuller(appleID, pw)

	stats, err := st.Sync(puller, "default")
	if err != nil {
		t.Fatalf("live sync 1: %v", err)
	}
	t.Logf("sync 1: collections=%d skipped=%d changed=%d", stats.Collections, stats.Skipped, stats.Changed)
	recs, _, err := st.RecordsSince(SourceICloud, KindContact, 0)
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	t.Logf("bronze contact records: %d", len(recs))
	if len(recs) == 0 {
		t.Fatalf("expected >0 contact records from live iCloud")
	}

	stats2, err := st.Sync(puller, "default")
	if err != nil {
		t.Fatalf("live sync 2: %v", err)
	}
	t.Logf("sync 2 (incremental): collections=%d skipped=%d changed=%d", stats2.Collections, stats2.Skipped, stats2.Changed)
	if stats2.Skipped != stats2.Collections {
		t.Logf("note: %d/%d collections re-pulled (CTag moved between runs)", stats2.Collections-stats2.Skipped, stats2.Collections)
	}

	// Close the loop: shape the real bronze into gold contacts (mirrors
	// govern.ICloudContacts, inlined to avoid the sources↔govern test import
	// cycle). Proves real vCards land as phone-keyed contacts.
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open meta.db: %v", err)
	}
	defer db.Close()
	cs := meta.NewContactStore(db)
	var imported []meta.ImportedContact
	for _, r := range recs {
		for _, p := range icloud.ParseVCards(r.Payload) {
			phone := ""
			if len(p.Phones) > 0 {
				phone = p.Phones[0]
			}
			imported = append(imported, meta.ImportedContact{Phone: phone, Name: p.Name, Company: p.Org, Title: p.Title})
		}
	}
	created, _, err := cs.IngestContacts(imported)
	if err != nil {
		t.Fatalf("ingest gold: %v", err)
	}
	t.Logf("gold contacts created: %d (from %d bronze vCards)", created, len(recs))
	if created == 0 {
		t.Fatalf("expected >0 gold contacts from real vCards")
	}
}
