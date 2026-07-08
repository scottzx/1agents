package sources

import (
	"os"
	"testing"
)

// TestXunjiLive drives the generic REST puller against the real 训记 API, proving
// the date-window walk + Bearer injection + res.trains extraction end to end.
// Guarded: runs only when XUNJI_TOKEN is set, so the normal suite stays offline.
//
//	XUNJI_TOKEN=xjllm_... go test ./internal/sources/ -run TestXunjiLive -v
func TestXunjiLive(t *testing.T) {
	token := os.Getenv("XUNJI_TOKEN")
	if token == "" {
		t.Skip("set XUNJI_TOKEN to run the live 训记 fetch test")
	}
	d := RESTDescriptor{
		Kind:         "xunji_train",
		Method:       "POST",
		Endpoint:     "/api_trains_for_llm_v2",
		Body:         map[string]any{"schema_version": "train_open_api_v2", "include_full_data": false},
		AuthScheme:   "bearer",
		ItemPath:     "res.trains", // NOTE: no successPath — the real API has no success field
		UIDField:     "localid",
		CursorFlavor: "date-window",
		DateParam:    "datestr",
		LookbackDays: 4, // covers 2026-07-06 (a known training day) from 2026-07-08
	}
	p := NewRESTPuller("xunji", "https://trains.xunjiapp.cn", []RESTDescriptor{d},
		func() (string, bool) { return token, true })

	colls, err := p.Discover("default")
	if err != nil || len(colls) != 1 {
		t.Fatalf("Discover = %v, %v", colls, err)
	}

	// Walk day-by-day exactly as Store.Sync does.
	cur := Cursor{}
	var all []RawRecord
	for i := 0; i < 12; i++ {
		recs, next, done, err := p.Pull("default", colls[0], cur)
		if err != nil {
			t.Fatalf("Pull: %v", err)
		}
		all = append(all, recs...)
		cur = next
		if done {
			break
		}
	}

	t.Logf("fetched %d training record(s) across the window", len(all))
	for _, r := range all {
		t.Logf("  uid=%s payload=%.120s", r.UID, r.Payload)
	}
	if len(all) == 0 {
		t.Fatal("expected at least one training record in the window (2026-07-06 has 1)")
	}
	found := false
	for _, r := range all {
		if r.UID == "1783328972762" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the known 2026-07-06 record (localid 1783328972762) in results")
	}
}
