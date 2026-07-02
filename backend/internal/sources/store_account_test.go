package sources

import "testing"

// commit one record under a given account so we can assert per-account scoping.
func seedRec(t *testing.T, st *Store, source, account, uid string) {
	t.Helper()
	rec := RawRecord{Kind: "contact", Collection: "c", UID: uid, ETag: uid, ContentType: "text/vcard", Payload: "x"}
	if _, err := st.CommitPage(source, account, []RawRecord{rec}, Cursor{Kind: "sync_token", Value: "t"}); err != nil {
		t.Fatalf("commit %s/%s: %v", account, uid, err)
	}
}

func TestListRecordsAccountScope(t *testing.T) {
	st := openSourcesStore(t)
	seedRec(t, st, "google", "acct-a", "a1")
	seedRec(t, st, "google", "acct-a", "a2")
	seedRec(t, st, "google", "acct-b", "b1")

	if recs, err := st.ListRecords("google", "acct-a", "contact", 0); err != nil || len(recs) != 2 {
		t.Fatalf("acct-a scope = %d,%v; want 2", len(recs), err)
	}
	if recs, err := st.ListRecords("google", "acct-b", "contact", 0); err != nil || len(recs) != 1 {
		t.Fatalf("acct-b scope = %d,%v; want 1", len(recs), err)
	}
	// Empty account spans all of the source's accounts.
	if recs, err := st.ListRecords("google", "", "contact", 0); err != nil || len(recs) != 3 {
		t.Fatalf("all-accounts = %d,%v; want 3", len(recs), err)
	}
}

func TestSummariesCarryAccount(t *testing.T) {
	st := openSourcesStore(t)
	seedRec(t, st, "google", "acct-a", "a1")
	seedRec(t, st, "google", "acct-b", "b1")

	sums, err := st.Summaries()
	if err != nil {
		t.Fatalf("summaries: %v", err)
	}
	byAcct := map[string]int{}
	for _, s := range sums {
		byAcct[s.AccountID] += s.Count
	}
	if byAcct["acct-a"] != 1 || byAcct["acct-b"] != 1 {
		t.Fatalf("expected 1 record per account, got %v", byAcct)
	}
}

func TestReassignAccount(t *testing.T) {
	st := openSourcesStore(t)
	seedRec(t, st, "icloud", "default", "c1")
	seedRec(t, st, "icloud", "default", "c2")

	if err := st.ReassignAccount("icloud", "default", "icloud-new"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if recs, _ := st.ListRecords("icloud", "default", "contact", 0); len(recs) != 0 {
		t.Fatalf("default should be empty after re-key, got %d", len(recs))
	}
	if recs, _ := st.ListRecords("icloud", "icloud-new", "contact", 0); len(recs) != 2 {
		t.Fatalf("icloud-new should hold 2 after re-key, got %d", len(recs))
	}
}
