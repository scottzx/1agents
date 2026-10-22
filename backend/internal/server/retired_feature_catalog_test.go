package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/config"
)

func TestRetiredFeatureCatalogRoutesReturnNotFound(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	cfg := config.Default()
	cfg.TmuxSession = ""
	cfg.WorkDir = t.TempDir()
	cfg.StaticDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg.StaticDir, "index.html"), []byte("existing app"), 0o600); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(cfg)
	request := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(`{}`))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(http.MethodGet, "/api/projects"); rec.Code != http.StatusOK {
		t.Fatalf("project API did not initialize: status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, route := range []struct{ method, target string }{
		{http.MethodGet, "/api/agent/feature-catalog"},
		{http.MethodPost, "/api/agent/feature-catalog"},
		{http.MethodPatch, "/api/agent/feature-catalog/retired-node"},
		{http.MethodPost, "/api/agent/feature-catalog/batch"},
		{http.MethodGet, "/api/agent/feature-catalog/export?format=json"},
		{http.MethodGet, "/api/agent/feature-catalog/gantt"},
	} {
		rec := request(route.method, route.target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status=%d body=%s", route.method, route.target, rec.Code, rec.Body.String())
		}
	}
}
