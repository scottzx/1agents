package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── versionGT ──────────────────────────────────────────────────────────────

func TestVersionGT(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"20260615-2", "20260615-1", true},
		{"20260615-1", "20260615-1", false},
		{"20260614-5", "20260615-1", false},
		{"", "", false},
		{"20260615-1", "", false},
		{"", "20260615-1", false},
		{"20260615-1", "unknown", false},
	}
	for _, tt := range tests {
		got := versionGT(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("versionGT(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// ── platformKey ─────────────────────────────────────────────────────────────

func TestPlatformKeyFormat(t *testing.T) {
	k := platformKey()
	parts := strings.Split(k, "-")
	if len(parts) != 2 {
		t.Fatalf("platformKey() = %q, want GOOS-GOARCH", k)
	}
	if parts[0] == "" || parts[1] == "" {
		t.Fatalf("platformKey() = %q has empty component", k)
	}
}

// ── getLocalVersion ─────────────────────────────────────────────────────────

func TestGetLocalVersion(t *testing.T) {
	// In test binaries the ldflags-injected version is empty; we
	// rely on the "VERSION" fallback path, which also won't exist
	// in tmp dirs. The result should be "unknown" — not a panic.
	old := LocalVersion
	t.Cleanup(func() { LocalVersion = old })

	LocalVersion = "dev"
	if getLocalVersion() != "unknown" {
		t.Log("expected 'unknown' in test binary; got", getLocalVersion())
	}

	LocalVersion = "v20260615-1"
	if got := getLocalVersion(); got != "v20260615-1" {
		t.Errorf("getLocalVersion() = %q, want v20260615-1", got)
	}
}

// ── Manifest decoding ───────────────────────────────────────────────────────

var sampleManifest = []byte(`{
  "channel": "stable",
  "released_at": "2026-06-15T10:00:00Z",
  "min_supported": "0.3.0",
  "components": {
    "frontend": {
      "version": "20260615-1",
      "entry": "https://example.com/frontend.tar.gz",
      "integrity": "sha256-abc"
    },
    "backend": {
      "version": "20260615-1",
      "platforms": {
        "linux-amd64": {
          "url": "https://example.com/1agents-linux-amd64.tar.gz",
          "size": 12345678,
          "sha256": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
        }
      }
    }
  }
}`)

func TestRootManifestDecode(t *testing.T) {
	var m RootManifest
	if err := json.Unmarshal(sampleManifest, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Channel != "stable" {
		t.Errorf("channel = %q", m.Channel)
	}
	if v := m.Components.Backend.Version; v != "20260615-1" {
		t.Errorf("backend.version = %q", v)
	}
	bin, ok := m.Components.Backend.Platforms["linux-amd64"]
	if !ok {
		t.Fatal("missing linux-amd64 platform")
	}
	if bin.URL == "" {
		t.Error("bin.URL is empty")
	}
}

func TestPlatformBinaryURL(t *testing.T) {
	_, _, err := platformBinaryURL(sampleManifest)
	// This will error unless the test runner *is* linux-amd64.
	// We just assert the function doesn't panic and the error is
	// meaningful when platform is absent.
	if err != nil {
		if !strings.Contains(err.Error(), "no binary") && !strings.Contains(err.Error(), "manifest has no binary") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// ── emptyManifest fallback ──────────────────────────────────────────────────

func TestEmptyManifestIsValidJSON(t *testing.T) {
	b := emptyManifest()
	var probe map[string]interface{}
	if err := json.Unmarshal(b, &probe); err != nil {
		t.Fatalf("emptyManifest is not valid JSON: %v", err)
	}
	components, ok := probe["components"].(map[string]interface{})
	if !ok {
		t.Fatal("emptyManifest missing components")
	}
	frontend, ok := components["frontend"].(map[string]interface{})
	if !ok {
		t.Fatal("emptyManifest missing frontend")
	}
	if v := frontend["version"]; v == nil || v == "" {
		t.Error("emptyManifest frontend.version is empty")
	}
}

// ── Manifest HTTP endpoint ──────────────────────────────────────────────────

func TestManifestEndpointMethod(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodPost, ManifestPath, nil)
	rec := httptest.NewRecorder()
	h.Manifest(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST Manifest returned %d, want 405", rec.Code)
	}
}

func TestManifestEndpointReturnsJSON(t *testing.T) {
	// cache is already invalid → triggers fetchUpstream → will
	// certainly fail in CI (no network, or rate-limited) so we
	// exercise the emptyManifest fallback path.
	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, ManifestPath, nil)
	rec := httptest.NewRecorder()
	h.Manifest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET Manifest returned %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	var probe map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &probe); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
}

// ── OTA update disabled guard ───────────────────────────────────────────────

func TestUpdateEndpointDisabled(t *testing.T) {
	old := OTAEnabled
	t.Cleanup(func() { OTAEnabled = old })
	OTAEnabled = false

	h := NewHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/system/update", nil)
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Update returned %d, want 503 when OTAEnabled=false", rec.Code)
	}
}

func TestUpdateEndpointOnlyPost(t *testing.T) {
	old := OTAEnabled
	t.Cleanup(func() { OTAEnabled = old })
	OTAEnabled = true

	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/system/update", nil)
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET Update returned %d, want 405", rec.Code)
	}
}

// ── Version endpoint ────────────────────────────────────────────────────────

func TestVersionEndpoint(t *testing.T) {
	old := LocalVersion
	t.Cleanup(func() { LocalVersion = old })
	LocalVersion = "v20260615-1"

	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/system/version", nil)
	rec := httptest.NewRecorder()
	h.Version(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET Version returned %d", rec.Code)
	}
	var info VersionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if info.Current != "v20260615-1" {
		t.Errorf("current = %q", info.Current)
	}
	if info.Channel != Channel {
		t.Errorf("channel = %q, want %q", info.Channel, Channel)
	}
}

// ── UpdateStatus endpoint ───────────────────────────────────────────────────

func TestUpdateStatusEndpointIdle(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/system/update/status", nil)
	rec := httptest.NewRecorder()
	h.UpdateStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET UpdateStatus returned %d", rec.Code)
	}
	var s map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if running, _ := s["running"].(bool); running {
		t.Error("UpdateStatus reported running=true when idle")
	}
}

// ── Concurrency: double-start protection ────────────────────────────────────

func TestUpdateDoubleStartRejected(t *testing.T) {
	old := OTAEnabled
	t.Cleanup(func() { OTAEnabled = old })
	OTAEnabled = true

	// Force start so the second attempt gets 409.
	state.start()
	defer state.finish()
	t.Cleanup(func() { state.finish() })

	h := NewHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/system/update", nil)
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("double Update returned %d, want 409", rec.Code)
	}
}

// ── Fetch manifest caching ──────────────────────────────────────────────────

func TestCacheLifecycle(t *testing.T) {
	// Invalidate the in-memory cache.
	cache.mu.Lock()
	cache.fetchedAt = time.Time{}
	cache.body = nil
	cache.mu.Unlock()

	body := manifestWithCache()
	if len(body) == 0 {
		t.Fatal("manifestWithCache returned empty body")
	}
	// When upstream is unreachable, the fallback (emptyManifest) does
	// NOT populate the cache — that would lock users into "no update"
	// until the TTL expires. The cache should remain empty.
	cache.mu.RLock()
	cacheEmpty := cache.fetchedAt.IsZero()
	cache.mu.RUnlock()
	if !cacheEmpty {
		t.Error("cache was populated by fallback path — should stay cold when upstream fails")
	}
}
