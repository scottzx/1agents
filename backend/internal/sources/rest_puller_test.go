package sources

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// dateWindowDesc builds a 训记-shaped POST descriptor pointing at srvURL.
func dateWindowDesc(srvURL string) RESTDescriptor {
	return RESTDescriptor{
		Kind:         "xunji_record",
		Domain:       "fitness",
		Label:        "训练记录",
		Method:       http.MethodPost,
		Endpoint:     srvURL + "/records",
		Body:         map[string]any{"schema_version": "train_open_api_v2", "include_full_data": false},
		AuthScheme:   "bearer",
		SuccessPath:  "success",
		ItemPath:     "res.trains",
		UIDField:     "localid",
		CursorFlavor: "date-window",
		DateParam:    "datestr",
		LookbackDays: 2,
	}
}

func TestRESTPuller_DateWindow(t *testing.T) {
	var gotDates []string
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		date, _ := gotBody["datestr"].(string)
		gotDates = append(gotDates, date)
		// One train per day, id derived from the date so uids differ.
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"res":     map[string]any{"trains": []map[string]any{{"localid": "t-" + date, "title": "胸"}}},
		})
	}))
	defer srv.Close()

	d := dateWindowDesc(srv.URL)
	p := NewRESTPuller("xunji", srv.URL, []RESTDescriptor{d}, func() (string, bool) { return "tok123", true })

	colls, err := p.Discover("default")
	if err != nil || len(colls) != 1 || colls[0].Kind != "xunji_record" {
		t.Fatalf("Discover = %v, %v", colls, err)
	}

	// Drive the day-by-day loop the way Store.Sync does: call Pull until done.
	cur := Cursor{}
	var all []RawRecord
	for i := 0; i < 10; i++ {
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

	// LookbackDays=2 ⇒ today-2, today-1, today = 3 days.
	if len(gotDates) != 3 {
		t.Fatalf("expected 3 day-requests, got %d: %v", len(gotDates), gotDates)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 records, got %d", len(all))
	}
	if gotAuth != "Bearer tok123" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	// Typed body field must survive as a JSON bool, not the string "false".
	if v, ok := gotBody["include_full_data"].(bool); !ok || v {
		t.Fatalf("include_full_data should be bool false, got %#v", gotBody["include_full_data"])
	}
	// Cursor advanced to today; a further Pull is a caught-up no-op.
	today := time.Now().Format("2006-01-02")
	if cur.Value != today {
		t.Fatalf("cursor = %q, want %q", cur.Value, today)
	}
	recs, _, done, err := p.Pull("default", colls[0], cur)
	if err != nil || !done || len(recs) != 0 {
		t.Fatalf("caught-up Pull = %v/%v/%v", recs, done, err)
	}
}

func TestRESTPuller_TooFrequentBacksOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "too frequent"})
	}))
	defer srv.Close()

	d := dateWindowDesc(srv.URL)
	d.TooFrequentPath = "error"
	p := NewRESTPuller("xunji", srv.URL, []RESTDescriptor{d}, func() (string, bool) { return "t", true })

	recs, next, done, err := p.Pull("default", Collection{Kind: d.Kind, ID: d.Kind}, Cursor{})
	if err != nil {
		t.Fatalf("too-frequent should not error, got %v", err)
	}
	if len(recs) != 0 || !done {
		t.Fatalf("expected empty done page, got recs=%d done=%v", len(recs), done)
	}
	// Cursor must NOT advance, so the next run retries the same day.
	if next.Value != "" {
		t.Fatalf("cursor should stay empty on back-off, got %q", next.Value)
	}
}

func TestBearerToken_RoundTrip(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	if BearerConfigured("xunji", "default") {
		t.Fatal("should start unconfigured")
	}
	if err := SaveBearerToken("xunji", "default", "secret-xyz"); err != nil {
		t.Fatalf("save: %v", err)
	}
	tok, ok, err := LoadBearerToken("xunji", "default")
	if err != nil || !ok || tok != "secret-xyz" {
		t.Fatalf("load = %q/%v/%v", tok, ok, err)
	}
	if !BearerConfigured("xunji", "default") {
		t.Fatal("should be configured after save")
	}
	DeleteBearerToken("xunji", "default")
	if BearerConfigured("xunji", "default") {
		t.Fatal("should be unconfigured after delete")
	}
}

func TestRESTRegistry(t *testing.T) {
	RegisterRESTDescriptor("demo", "https://api.demo.test", RESTDescriptor{Kind: "demo_item", Domain: "misc", Label: "Item"})
	if d, ok := RESTDescriptorFor("demo", "demo_item"); !ok || d.Label != "Item" {
		t.Fatalf("descriptor not registered: %v/%v", d, ok)
	}
	if b, ok := RESTBaseURL("demo"); !ok || b != "https://api.demo.test" {
		t.Fatalf("base url = %q/%v", b, ok)
	}
	items := restCatalogItems("demo")
	if len(items) != 1 || !items[0].Implemented || items[0].Kind != "demo_item" {
		t.Fatalf("catalog items = %v", items)
	}
	// CatalogFor must surface the REST source like a built-in.
	if CatalogItemFor("demo", "demo_item") == nil {
		t.Fatal("CatalogItemFor should find REST kind")
	}
}
