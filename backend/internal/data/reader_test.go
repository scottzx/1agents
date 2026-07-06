package data

import "testing"

// TestListSilverAndSummary seeds rows in two source tables of the same domain and
// checks the viewer reader: a domain view unions its source tables newest-first
// as (key,value) fields, source-filters to one table, and the summary rolls up
// by (domain, source).
func TestListSilverAndSummary(t *testing.T) {
	st := openTemp(t)

	if _, err := st.UpsertFeishuMessages([]SilverFeishuMessage{
		{ExternalID: "om_1", ChatID: "oc_1", SenderOpenID: "ou_a", BodyText: "hi", CreateTime: 100, UpdatedAt: 10},
	}); err != nil {
		t.Fatalf("seed feishu messages: %v", err)
	}
	if _, err := st.UpsertMicrosoftMail([]SilverMicrosoftMail{
		{ExternalID: "ms_1", Subject: "Hello", FromAddr: "a@b.com", UpdatedAt: 20},
	}); err != nil {
		t.Fatalf("seed ms mail: %v", err)
	}

	// Domain view unions 飞书 + microsoft, newest (higher updated_at) first.
	rows, err := st.ListSilver("messages", "", 0)
	if err != nil {
		t.Fatalf("ListSilver: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].UID != "ms_1" || rows[0].Collection != "microsoft" {
		t.Errorf("row0 = uid %q source %q, want ms_1/microsoft", rows[0].UID, rows[0].Collection)
	}
	// updated_at is the envelope (FetchedAt), never a duplicate field.
	for _, f := range rows[0].Fields {
		if f.Key == "updated_at" {
			t.Errorf("updated_at should not be a field")
		}
	}

	// Source filter narrows to one table.
	fs, _ := st.ListSilver("messages", "feishu", 0)
	if len(fs) != 1 || fs[0].UID != "om_1" {
		t.Fatalf("source-filtered = %+v, want just om_1", fs)
	}

	// Summary groups by (domain, source).
	sum, err := st.SilverSummary()
	if err != nil {
		t.Fatalf("SilverSummary: %v", err)
	}
	got := map[string]int{}
	for _, s := range sum {
		got[s.Domain+"/"+s.Source] = s.Count
	}
	if got["messages/feishu"] != 1 || got["messages/microsoft"] != 1 {
		t.Fatalf("summary = %+v, want messages/feishu=1 messages/microsoft=1", got)
	}

	// Unknown domain is rejected, not silently empty.
	if _, err := st.ListSilver("bogus", "", 0); err == nil {
		t.Errorf("expected error for unknown domain")
	}
}
