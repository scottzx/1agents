package icloud

import (
	"os"
	"testing"
)

// TestLiveFetch hits the real iCloud CardDAV server. Gated by credentials in env
// (ICLOUD_APPLE_ID + ICLOUD_APP_PASSWORD) so it stays off in normal test runs.
func TestLiveFetch(t *testing.T) {
	id, pw := os.Getenv("ICLOUD_APPLE_ID"), os.Getenv("ICLOUD_APP_PASSWORD")
	if id == "" || pw == "" {
		t.Skip("set ICLOUD_APPLE_ID + ICLOUD_APP_PASSWORD to run the live fetch")
	}
	contacts, err := NewClient(id, pw).FetchContacts()
	if err != nil {
		t.Fatalf("FetchContacts: %v", err)
	}
	t.Logf("fetched %d contacts", len(contacts))
	for i, c := range contacts {
		if i >= 3 {
			break
		}
		t.Logf("  [%d] name=%q phones=%v emails=%v org=%q", i, c.Name, c.Phones, c.Emails, c.Org)
	}
}
