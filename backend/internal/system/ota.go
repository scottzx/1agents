// OTA manifest endpoint — proxies the root manifest published on the
// project's GitHub Releases. The frontend OTA checker polls this URL;
// the result is cached in-memory for 5 minutes to avoid hammering the
// GitHub Releases API on hot page loads.
//
// Week 1 scope: passive proxy + minimal fallback manifest.
// Week 2 will add: GitHub-Authenticated backend self-update, replacing
// the existing `npm install -g` flow (see ota-architecture.md).

package system

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Package vars overridable by main.go / tests.
//
//   - MirrorBaseURL is the CDN OTA mirror; when non-empty it is the
//     PRIMARY upstream. Its /manifest.json is tried first, and the binary
//     URLs it contains point back at the CDN. Set it to the CDN base
//     (e.g. "https://ota.example.com") once the COS+CDN bucket is
//     provisioned; "" means GitHub-only.
//   - Repo is the GitHub slug used for the FALLBACK upstream when the
//     mirror is unreachable. Set to "" to disable the GitHub fallback.
var (
	MirrorBaseURL = "https://1agents-ota-1258742922.cos.ap-shanghai.myqcloud.com" // COS regional OTA mirror (no CDN acceleration)
	Repo          = "scottzx/1Agents"
	LocalVersion  = "dev" // set from cmd/backend/main.go via ldflags
	OTAEnabled    = false // set from cmd/backend/main.go; false in desktop/Docker mode
)

const (
	manifestURL  = "https://github.com/%s/releases/latest/download/manifest.json"
	cacheTTL     = 5 * time.Minute
	upstreamTO   = 8 * time.Second
	ManifestPath = "/api/ota/manifest"
)

// ── Manifest types (shared by ota.go and system.go) ────────────────────────

// RootManifest mirrors the JSON structure published as
// releases/latest/download/manifest.json on GitHub Releases.
// See docs/ota-architecture.md §4.1 for the canonical shape.
type RootManifest struct {
	Channel      string            `json:"channel"`
	ReleasedAt   string            `json:"released_at"`
	MinSupported string            `json:"min_supported"`
	Components   Components        `json:"components"`
	Previous     []PreviousRelease `json:"previous"`
}

type Components struct {
	Frontend FrontendComponent `json:"frontend"`
	Backend  BackendComponent  `json:"backend"`
}

type FrontendComponent struct {
	Version   string `json:"version"`
	Entry     string `json:"entry"`
	Integrity string `json:"integrity"`
}

type BackendComponent struct {
	Version   string                    `json:"version"`
	Platforms map[string]PlatformBinary `json:"platforms"`
}

type PlatformBinary struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type PreviousRelease struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// otaCache holds the most recently fetched manifest and when it was
// last refreshed. We intentionally do NOT persist this to disk —
// transient outages should not lock users out of a fresh check.
type otaCache struct {
	mu        sync.RWMutex
	body      []byte
	fetchedAt time.Time
}

var cache otaCache

// emptyManifest is what we serve when the upstream is unreachable.
// `components.frontend.version` is set to the local binary's version
// string (injected via -ldflags) so the checker sees hasUpdate=false
// rather than erroring out.
func emptyManifest() []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"channel":       "stable",
		"released_at":   time.Now().UTC().Format(time.RFC3339),
		"min_supported": "0.0.0",
		"components": map[string]interface{}{
			"frontend": map[string]string{
				"version":   LocalVersion,
				"entry":     "",
				"integrity": "",
			},
			"backend": map[string]interface{}{
				"version":   LocalVersion,
				"platforms": map[string]interface{}{},
			},
		},
		"previous": []interface{}{},
	})
	return b
}

// upstreamSources returns the manifest URLs to try, in priority order:
// self-hosted mirror first, GitHub Releases as fallback. Either can be
// disabled by setting its package var to "".
func upstreamSources() []string {
	var srcs []string
	if MirrorBaseURL != "" {
		srcs = append(srcs, strings.TrimRight(MirrorBaseURL, "/")+"/manifest.json")
	}
	if Repo != "" {
		srcs = append(srcs, fmt.Sprintf(manifestURL, Repo))
	}
	return srcs
}

// fetchManifestFrom fetches and validates a manifest from a single URL.
func fetchManifestFrom(url string) ([]byte, error) {
	client := &http.Client{Timeout: upstreamTO}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s returned %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	// Validate that it's at least shaped like a manifest — defensive
	// against 200-with-HTML-404-page responses from CDN edges.
	var probe map[string]interface{}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, fmt.Errorf("upstream body is not valid JSON: %w", err)
	}
	if _, ok := probe["components"]; !ok {
		return nil, fmt.Errorf("upstream body missing 'components' field")
	}
	return body, nil
}

// fetchUpstream pulls the latest manifest, preferring the self-hosted
// mirror and falling back to GitHub Releases. Any network/parse error
// from all sources is returned to the caller; the caller decides
// whether to fall back to the cached copy or the empty manifest.
func fetchUpstream() ([]byte, error) {
	sources := upstreamSources()
	if len(sources) == 0 {
		return nil, fmt.Errorf("no OTA upstream configured")
	}
	var lastErr error
	for _, url := range sources {
		body, err := fetchManifestFrom(url)
		if err == nil {
			return body, nil
		}
		log.Printf("[ota] upstream %s failed: %v", url, err)
		lastErr = err
	}
	return nil, lastErr
}

// manifestWithCache returns the manifest body to serve. The order of
// preference is: fresh cache → upstream → stale cache → empty manifest.
func manifestWithCache() []byte {
	cache.mu.RLock()
	fresh := !cache.fetchedAt.IsZero() && time.Since(cache.fetchedAt) < cacheTTL
	stale := !cache.fetchedAt.IsZero() && time.Since(cache.fetchedAt) >= cacheTTL
	cached := cache.body
	cache.mu.RUnlock()

	if fresh {
		return cached
	}

	body, err := fetchUpstream()
	if err != nil {
		log.Printf("[ota] upstream fetch failed: %v (serving %s)", err, map[bool]string{true: "stale cache", false: "empty manifest"}[stale])
		if stale {
			return cached
		}
		return emptyManifest()
	}

	cache.mu.Lock()
	cache.body = body
	cache.fetchedAt = time.Now()
	cache.mu.Unlock()
	return body
}

// Manifest handles GET /api/ota/manifest. Response shape mirrors the
// root manifest published on GitHub Releases (see ota-architecture.md
// §4.1). Always returns 200 — clients distinguish "no update" by
// reading components.frontend.version themselves.
func (h *Handler) Manifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Tell intermediaries not to cache: clients rely on the in-process
	// cache and on ETag-like freshness from `released_at`.
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	_, _ = w.Write(manifestWithCache())
}
