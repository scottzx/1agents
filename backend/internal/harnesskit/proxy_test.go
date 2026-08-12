package harnesskit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/config"
	"github.com/scottzx/1Agents/backend/internal/supervisor"
)

type fakeRuntime struct {
	mu       sync.RWMutex
	status   supervisor.HarnessKitStatus
	endpoint string
	token    string
	ready    bool
}

func (f *fakeRuntime) Status() supervisor.HarnessKitStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.status
}

func (f *fakeRuntime) Endpoint() (string, string, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.endpoint, f.token, f.ready
}

func TestStatusDoesNotExposeCredentials(t *testing.T) {
	runtime := &fakeRuntime{
		status: supervisor.HarnessKitStatus{
			Mode:          "supervised",
			State:         "ready",
			Ready:         true,
			Port:          43210,
			LastChangedAt: time.Now(),
		},
		endpoint: "http://127.0.0.1:43210",
		token:    "super-secret-token",
		ready:    true,
	}
	rec := httptest.NewRecorder()
	NewHandler(runtime).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesskit/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), runtime.token) || strings.Contains(rec.Body.String(), runtime.endpoint) {
		t.Fatalf("status response leaked private connection details: %s", rec.Body.String())
	}
	var got supervisor.HarnessKitStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Ready || got.State != "ready" || got.Port != 43210 {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestAllowedRouteInjectsTokenAndRewritesPath(t *testing.T) {
	const daemonToken = "daemon-only-token"
	var gotMethod, gotPath, gotQuery, gotAuth, gotCookie, gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Set-Cookie", "hk=secret")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	runtime := &fakeRuntime{
		status:   supervisor.HarnessKitStatus{State: "ready", Ready: true},
		endpoint: upstream.URL,
		token:    daemonToken,
		ready:    true,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/harnesskit/list_extensions?page=2", strings.NewReader(`{"kind":"skill"}`))
	req.Header.Set("Authorization", "Bearer browser-token")
	req.Header.Set("Cookie", "session=browser")
	rec := httptest.NewRecorder()
	NewHandler(runtime).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/list_extensions" || gotQuery != "page=2" {
		t.Fatalf("upstream request = %s %s?%s", gotMethod, gotPath, gotQuery)
	}
	if gotAuth != "Bearer "+daemonToken {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotCookie != "" {
		t.Fatalf("browser cookie forwarded upstream: %q", gotCookie)
	}
	if gotBody != `{"kind":"skill"}` {
		t.Fatalf("body = %q", gotBody)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("unsafe upstream headers survived: %v", rec.Header())
	}
}

func TestProxyAllowsStaticUIAssetGETRoutes(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	runtime := &fakeRuntime{
		status:   supervisor.HarnessKitStatus{State: "ready", Ready: true},
		endpoint: upstream.URL,
		token:    "token",
		ready:    true,
	}
	handler := NewHandler(runtime)

	for reqPath, expectedUpstream := range map[string]string{
		"/api/harnesskit/":                  "/",
		"/api/harnesskit/index.html":         "/index.html",
		"/api/harnesskit/assets/index.js":   "/assets/index.js",
		"/api/harnesskit/assets/style.css":  "/assets/style.css",
		"/api/harnesskit/favicon.png":       "/favicon.png",
	} {
		gotPath = ""
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, reqPath, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want %d", reqPath, rec.Code, http.StatusOK)
		}
		if gotPath != expectedUpstream {
			t.Errorf("%s upstream path = %q, want %q", reqPath, gotPath, expectedUpstream)
		}
	}
}

func TestProxyFailsClosedForUnknownAndHostOnlyRoutes(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	runtime := &fakeRuntime{
		status:   supervisor.HarnessKitStatus{State: "ready", Ready: true},
		endpoint: upstream.URL,
		token:    "token",
		ready:    true,
	}
	handler := NewHandler(runtime)
	for _, path := range []string{
		"/api/harnesskit/future_upstream_route",
		"/api/harnesskit/update_agent_path",
		"/api/harnesskit/add_project",
		"/api/harnesskit/install_from_local",
		"/api/harnesskit/export_kit",
		"/api/harnesskit/import_kit",
		"/api/harnesskit/uninstall_cli_binary",
		"/api/harnesskit/list_skill_files",
		"/api/harnesskit/count_project_extensions",
		"/api/harnesskit/create_kit",
		"/api/harnesskit/update_kit",
		"/api/harnesskit/preview_kit_project_conflicts",
		"/api/harnesskit/toggle_extension",
		"/api/harnesskit/delete_extension",
		"/api/harnesskit/scan_and_sync",
		"/api/harnesskit/update_tags",
		"/api/harnesskit/set_agent_enabled",
		"/api/harnesskit/run_audit",
		"/api/harnesskit/update_extension",
		"/api/harnesskit/delete_kit",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s status = %d, want %d", path, rec.Code, http.StatusForbidden)
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("denied routes reached upstream %d times", upstreamCalls)
	}
}

func TestProxyRejectsWrongMethodAndWebSocket(t *testing.T) {
	runtime := &fakeRuntime{
		status: supervisor.HarnessKitStatus{State: "ready", Ready: true},
		ready:  true,
	}
	handler := NewHandler(runtime)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesskit/list_extensions", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET allowlisted POST route status = %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/harnesskit/list_extensions", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("WebSocket status = %d", rec.Code)
	}
}

func TestProxyReturnsStructuredUnavailable(t *testing.T) {
	runtime := &fakeRuntime{
		status: supervisor.HarnessKitStatus{
			Mode:      "supervised",
			State:     "degraded",
			LastError: "binary not found",
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/harnesskit/list_extensions", nil)
	req.Header.Set("X-Correlation-ID", "test-correlation")
	rec := httptest.NewRecorder()
	NewHandler(runtime).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "harnesskit_unavailable" || body.CorrelationID != "test-correlation" {
		t.Fatalf("unexpected error response: %+v", body)
	}
	if body.HarnessKit == nil || body.HarnessKit.State != "degraded" {
		t.Fatalf("missing degraded state: %+v", body)
	}
}

func TestProxyReplacesUnsafeCorrelationID(t *testing.T) {
	runtime := &fakeRuntime{status: supervisor.HarnessKitStatus{State: "degraded"}}
	req := httptest.NewRequest(http.MethodPost, "/api/harnesskit/list_extensions", nil)
	req.Header.Set("X-Correlation-ID", "contains spaces and secrets")
	rec := httptest.NewRecorder()
	NewHandler(runtime).ServeHTTP(rec, req)

	got := rec.Header().Get("X-Correlation-ID")
	if got == "" || got == "contains spaces and secrets" {
		t.Fatalf("unsafe correlation ID was not replaced: %q", got)
	}
}

func TestRealHarnessKitSupervisedProxy(t *testing.T) {
	binary := os.Getenv("HARNESSKIT_INTEGRATION_BIN")
	if binary == "" {
		t.Skip("set HARNESSKIT_INTEGRATION_BIN to run the real daemon smoke test")
	}

	cfg := config.Default()
	cfg.HarnessKitBinaryPath = binary
	cfg.HarnessKitDataDir = t.TempDir()
	cfg.HarnessKitPort = 0
	cfg.RestartDelay = 10 * time.Millisecond
	cfg.MaxRestarts = 1

	ctx, cancel := context.WithCancel(context.Background())
	sup := supervisor.NewHarnessKit(cfg)
	sup.Start(ctx)
	defer func() {
		cancel()
		select {
		case <-sup.Done():
		case <-time.After(5 * time.Second):
			t.Error("real HarnessKit supervisor did not stop")
		}
	}()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && !sup.Status().Ready {
		if sup.Status().State == "degraded" {
			t.Fatalf("real HarnessKit degraded before readiness: %+v", sup.Status())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sup.Status().Ready {
		t.Fatalf("real HarnessKit did not become ready: %+v", sup.Status())
	}

	endpoint, _, ready := sup.Endpoint()
	if !ready {
		t.Fatal("ready supervisor did not expose its private endpoint")
	}
	resp, err := http.Get(endpoint + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("direct unauthenticated health status = %d", resp.StatusCode)
	}

	rec := httptest.NewRecorder()
	NewHandler(sup).ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/api/harnesskit/list_extensions",
		strings.NewReader(`{}`),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("proxied list_extensions status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var extensions []json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &extensions); err != nil {
		t.Fatalf("proxied response is not an extension array: %v; body=%s", err, rec.Body.String())
	}
}
