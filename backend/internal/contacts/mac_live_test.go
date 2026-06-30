//go:build darwin

package contacts

import (
	"os"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// TestLivePull exercises the real pull end-to-end into an isolated temp store:
// iCloud contacts via CardDAV (credentials from ICLOUD_APPLE_ID +
// ICLOUD_APP_PASSWORD) and iMessage from chat.db (needs Full Disk Access). Gated
// by ONEAGENTS_MAC_LIVE=1 since it needs real credentials / OS grants.
func TestLivePull(t *testing.T) {
	if os.Getenv("ONEAGENTS_MAC_LIVE") != "1" {
		t.Skip("set ONEAGENTS_MAC_LIVE=1 to run the real pull")
	}
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	h, err := NewHandlerDefault()
	if err != nil {
		t.Fatal(err)
	}

	// iCloud 通讯录 via CardDAV.
	id, pw := os.Getenv("ICLOUD_APPLE_ID"), os.Getenv("ICLOUD_APP_PASSWORD")
	if id != "" && pw != "" {
		people, ferr := icloud.NewClient(id, pw).FetchContacts()
		if ferr != nil {
			t.Fatalf("icloud fetch: %v", ferr)
		}
		imported := make([]meta.ImportedContact, 0, len(people))
		for _, p := range people {
			phone := ""
			if len(p.Phones) > 0 {
				phone = p.Phones[0]
			}
			imported = append(imported, meta.ImportedContact{
				Phone: phone, Name: p.Name, Company: p.Org, Title: p.Title,
			})
		}
		c, u, ierr := h.cs.IngestContacts(imported)
		if ierr != nil {
			t.Fatalf("ingest contacts: %v", ierr)
		}
		t.Logf("iCloud 通讯录: fetched=%d created=%d updated=%d", len(people), c, u)
	} else {
		t.Log("iCloud: set ICLOUD_APPLE_ID + ICLOUD_APP_PASSWORD to test CardDAV")
	}

	// iMessage via chat.db.
	fetched, inserted, wm, err := h.SyncIMessage()
	if err != nil {
		t.Fatalf("imessage sync: %v", err)
	}
	t.Logf("iMessage: fetched=%d inserted=%d watermark=%d", fetched, inserted, wm)
}
