package ingest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/sources"
)

const pushConnectorYAML = `
vendor: agent_insights_http
label: Agent 洞察(HTTP 测试)
collections:
  - kind: sentiment
    domain: insights
    label: 情感分析
    transport: push
    uidField: id
    schema:
      - name: id
        type: string
        required: true
      - name: score
        type: number
    silver:
      table: silver_agent_sentiment_http
      domain: insights
`

// TestHandlePush_HotAddAuthCommit drives the real HTTP receiver end-to-end: hot-add
// a push connector (no restart), reject unauthenticated/invalid pushes, then accept
// an authenticated one and confirm it landed in bronze.
func TestHandlePush_HotAddAuthCommit(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())

	h, err := NewHandlerDefault()
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	if _, err := h.AddConnector([]byte(pushConnectorYAML)); err != nil {
		t.Fatalf("add push connector: %v", err)
	}
	const source, kind = "agent_insights_http", "sentiment"
	if !sources.IsPushSource(source) {
		t.Fatalf("push source not registered")
	}

	// The UI's push-info endpoint exposes the declared schema for this source.
	infoRR := httptest.NewRecorder()
	h.HandlePushInfo(infoRR, httptest.NewRequest(http.MethodGet, "/api/sources/"+source+"/push", nil))
	if infoRR.Code != http.StatusOK {
		t.Fatalf("push info = %d, want 200", infoRR.Code)
	}
	if !strings.Contains(infoRR.Body.String(), `"kind":"sentiment"`) ||
		!strings.Contains(infoRR.Body.String(), `"name":"id"`) {
		t.Fatalf("push info missing schema: %s", infoRR.Body.String())
	}

	account := h.manifestAccountID(source)
	const key = "super-secret-push-key"
	if err := sources.SaveBearerToken(source, account, key); err != nil {
		t.Fatalf("save push key: %v", err)
	}

	path := "/api/data/push/" + source + "/" + kind
	body := `[{"id":"s-1","score":0.9},{"id":"s-2","score":0.2}]`

	// No key → 401.
	if code := doPush(h, path, body, ""); code != http.StatusUnauthorized {
		t.Fatalf("no-auth push = %d, want 401", code)
	}
	// Wrong key → 401.
	if code := doPush(h, path, body, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("bad-auth push = %d, want 401", code)
	}
	// Missing required field → 422, nothing committed.
	if code := doPush(h, path, `{"score":0.5}`, key); code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid push = %d, want 422", code)
	}
	// Valid, authenticated → 200, committed 2.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	h.HandlePush(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid push = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	var resp struct {
		Received  int `json:"received"`
		Committed int `json:"committed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Received != 2 || resp.Committed != 2 {
		t.Fatalf("resp = %+v, want received=2 committed=2", resp)
	}

	// Bronze holds the pushed records verbatim (the retention hook).
	recs, err := h.bronze.ListRecords(source, "", kind, 10)
	if err != nil {
		t.Fatalf("list bronze: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("bronze has %d records, want 2", len(recs))
	}

	// Re-pushing the identical batch is idempotent (0 newly committed).
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+key)
	h.HandlePush(rr2, req2)
	var resp2 struct {
		Committed int `json:"committed"`
	}
	_ = json.Unmarshal(rr2.Body.Bytes(), &resp2)
	if resp2.Committed != 0 {
		t.Fatalf("re-push committed %d, want 0", resp2.Committed)
	}
}

func doPush(h *Handler, path, body, key string) int {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	h.HandlePush(rr, req)
	return rr.Code
}
