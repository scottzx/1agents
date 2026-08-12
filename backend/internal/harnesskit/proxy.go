package harnesskit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/supervisor"
)

const proxyPrefix = "/api/harnesskit"

type Runtime interface {
	Status() supervisor.HarnessKitStatus
	Endpoint() (baseURL, token string, ready bool)
}

type routePermission struct {
	method string
	path   string
}

// browserRoutes is intentionally exact and version-pinned. New HarnessKit
// endpoints are denied until their input and filesystem authority are reviewed.
//
// Read / inventory routes used by the embedded SPA shell are allowlisted so
// the iframe can boot. Mutating install/delete/export routes stay denied.
var browserRoutes = map[routePermission]struct{}{
	{http.MethodPost, "/api/server_info"}:                {},
	{http.MethodPost, "/api/get_dashboard_stats"}:        {},
	{http.MethodPost, "/api/list_extensions"}:            {},
	{http.MethodPost, "/api/list_agents"}:                {},
	{http.MethodPost, "/api/list_audit_results"}:         {},
	{http.MethodPost, "/api/search_marketplace"}:         {},
	{http.MethodPost, "/api/trending_marketplace"}:       {},
	{http.MethodPost, "/api/list_cli_marketplace"}:       {},
	{http.MethodPost, "/api/fetch_skill_preview"}:        {},
	{http.MethodPost, "/api/fetch_cli_readme"}:           {},
	{http.MethodPost, "/api/fetch_skill_audit"}:          {},
	{http.MethodPost, "/api/list_projects"}:              {},
	{http.MethodPost, "/api/list_kits"}:                  {},
	{http.MethodPost, "/api/get_kit_details"}:            {},
	{http.MethodPost, "/api/list_kit_asset_candidates"}:  {},
	{http.MethodPost, "/api/get_extension_content"}:      {},
	{http.MethodPost, "/api/get_all_tags"}:               {},
	{http.MethodPost, "/api/get_all_packs"}:              {},
	{http.MethodPost, "/api/list_agent_configs"}:         {},
	{http.MethodPost, "/api/list_hermes_categories"}:     {},
	{http.MethodPost, "/api/get_cli_with_children"}:      {},
	{http.MethodPost, "/api/get_skill_locations"}:        {},
	{http.MethodPost, "/api/read_config_file_preview"}:   {},
	// Inventory refresh used on SPA boot (writes only HarnessKit's own DB).
	{http.MethodPost, "/api/scan_and_sync"}:              {},
	{http.MethodPost, "/api/get_cached_update_statuses"}: {},
	{http.MethodPost, "/api/check_updates"}:              {},
	{http.MethodPost, "/api/list_skill_files"}:           {},
	{http.MethodPost, "/api/count_project_extensions"}:   {},
	{http.MethodPost, "/api/list_project_install_records"}: {},
}

// Handler exposes the credential-free status endpoint and the fail-closed
// browser proxy.
type Handler struct {
	runtime Runtime
	timeout time.Duration
}

func NewHandler(runtime Runtime) *Handler {
	return &Handler{runtime: runtime, timeout: 60 * time.Second}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	correlationID := requestCorrelationID(r)
	w.Header().Set("X-Correlation-ID", correlationID)

	if r.URL.Path == proxyPrefix+"/status" {
		h.serveStatus(w, r, correlationID)
		return
	}
	if !strings.HasPrefix(r.URL.Path, proxyPrefix+"/") {
		writeError(w, http.StatusNotFound, "route_not_found", "HarnessKit route not found", correlationID, nil)
		return
	}
	if isWebSocketUpgrade(r) {
		writeError(w, http.StatusBadRequest, "websocket_not_supported", "HarnessKit WebSocket proxying is disabled", correlationID, nil)
		return
	}

	relPath := strings.TrimPrefix(r.URL.Path, proxyPrefix)
	var upstreamPath string
	if isStaticUIAsset(r.Method, relPath) {
		upstreamPath = relPath
		if upstreamPath == "" {
			upstreamPath = "/"
		}
	} else {
		upstreamPath = "/api" + relPath
		if _, ok := browserRoutes[routePermission{method: r.Method, path: upstreamPath}]; !ok {
			writeError(w, http.StatusForbidden, "route_not_allowed", "HarnessKit route is not available through the embedded boundary", correlationID, nil)
			return
		}
	}

	baseURL, token, ready := h.runtime.Endpoint()
	if !ready {
		status := h.runtime.Status()
		writeError(w, http.StatusServiceUnavailable, "harnesskit_unavailable", "HarnessKit is not ready", correlationID, &status)
		return
	}
	target, err := url.Parse(baseURL)
	if err != nil || target.Scheme != "http" || target.Host == "" {
		status := h.runtime.Status()
		writeError(w, http.StatusServiceUnavailable, "harnesskit_unavailable", "HarnessKit endpoint is invalid", correlationID, &status)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	r = r.WithContext(ctx)

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = upstreamPath
		req.URL.RawPath = ""
		req.Host = target.Host
		req.Header.Del("Cookie")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Correlation-ID", correlationID)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Set-Cookie")
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Credentials")
		// HarnessKit SPA is built with absolute root paths (/assets/…, /favicon.png).
		// When framed under /api/harnesskit/ those resolve against the 1agents
		// host root and hit the main SPA fallback (text/html MIME), so rewrite
		// them onto the proxy prefix before the browser parses the document.
		if err := rewriteHarnessKitHTMLAssetPaths(resp); err != nil {
			return err
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("[harnesskit-proxy] correlation=%s upstream request failed: %v", correlationID, err)
		status := h.runtime.Status()
		writeError(w, http.StatusBadGateway, "harnesskit_upstream_error", "HarnessKit request failed", correlationID, &status)
	}
	proxy.ServeHTTP(w, r)
}

func (h *Handler) serveStatus(w http.ResponseWriter, r *http.Request, correlationID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed", correlationID, nil)
		return
	}
	status := h.runtime.Status()
	baseURL, token, ready := h.runtime.Endpoint()
	webURL := ""
	if ready && baseURL != "" && token != "" {
		webURL = proxyPrefix + "/"
	}
	resp := map[string]any{
		"mode":          status.Mode,
		"state":         status.State,
		"ready":         status.Ready,
		"port":          status.Port,
		"restartCount":  status.RestartCount,
		"maxRestarts":   status.MaxRestarts,
		"lastChangedAt": status.LastChangedAt,
		"webUrl":        webURL,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func isStaticUIAsset(method, relPath string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if relPath == "" || relPath == "/" || relPath == "/index.html" || relPath == "/favicon.png" || relPath == "/favicon.ico" || relPath == "/vite.svg" {
		return true
	}
	if strings.HasPrefix(relPath, "/assets/") {
		return true
	}
	return false
}

// harnessKitEmbedBootstrap is injected into the SPA shell so standalone
// transport calls (POST /api/{command}) are rewritten to the proxy prefix.
// The baked-in SPA uses apiBase="/api"; under /api/harnesskit/ those hit the
// 1agents host and 404. Custom-element embeds already pass api-base correctly.
const harnessKitEmbedBootstrap = `<script data-1agents-hk-embed="1">
(function () {
  var P = "` + proxyPrefix + `";
  if (window.__1AGENTS_HK_FETCH_PATCHED__) return;
  window.__1AGENTS_HK_FETCH_PATCHED__ = true;
  var orig = window.fetch;
  if (typeof orig !== "function") return;
  function rewriteUrl(url) {
    if (typeof url !== "string" || url.length === 0) return url;
    // Path-absolute /api/cmd → /api/harnesskit/cmd
    if (url.charAt(0) === "/" && url.indexOf("/api/") === 0 && url.indexOf(P + "/") !== 0 && url !== P) {
      return P + url.slice(4);
    }
    // Same-origin absolute URL
    if (url.indexOf("://") !== -1) {
      try {
        var u = new URL(url, location.href);
        if (u.origin === location.origin && u.pathname.indexOf("/api/") === 0 && u.pathname.indexOf(P + "/") !== 0) {
          u.pathname = P + u.pathname.slice(4);
          return u.toString();
        }
      } catch (e) {}
    }
    return url;
  }
  window.fetch = function (input, init) {
    try {
      if (typeof input === "string") {
        input = rewriteUrl(input);
      } else if (input && typeof input.url === "string") {
        var next = rewriteUrl(input.url);
        if (next !== input.url) input = new Request(next, input);
      }
    } catch (e) {}
    return orig.call(this, input, init);
  };
})();
</script>`

// rewriteHarnessKitHTMLAssetPaths rewrites absolute SPA asset URLs so the
// browser loads them through the /api/harnesskit reverse proxy instead of the
// 1agents host root. Only touches text/html responses (index.html / SPA shell).
func rewriteHarnessKitHTMLAssetPaths(resp *http.Response) error {
	if resp == nil || resp.Body == nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "text/html") {
		return nil
	}
	// Don't attempt to rewrite compressed payloads; hk serve returns plain HTML.
	if enc := strings.TrimSpace(resp.Header.Get("Content-Encoding")); enc != "" && !strings.EqualFold(enc, "identity") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return err
	}

	// Quote-aware replacements for Vite-built modulepreload/script/link tags.
	// Also cover favicon/root static files that the SPA shell references.
	replacements := []struct{ old, new string }{
		{`"/assets/`, `"` + proxyPrefix + `/assets/`},
		{`'/assets/`, `'` + proxyPrefix + `/assets/`},
		{`"/favicon.png"`, `"` + proxyPrefix + `/favicon.png"`},
		{`'/favicon.png'`, `'` + proxyPrefix + `/favicon.png'`},
		{`"/favicon.ico"`, `"` + proxyPrefix + `/favicon.ico"`},
		{`'/favicon.ico'`, `'` + proxyPrefix + `/favicon.ico'`},
		{`"/vite.svg"`, `"` + proxyPrefix + `/vite.svg"`},
		{`'/vite.svg'`, `'` + proxyPrefix + `/vite.svg'`},
	}
	rewritten := body
	for _, rep := range replacements {
		rewritten = bytes.ReplaceAll(rewritten, []byte(rep.old), []byte(rep.new))
	}

	// Patch fetch before module scripts run so POST /api/* hits the proxy.
	if !bytes.Contains(rewritten, []byte(`data-1agents-hk-embed="1"`)) {
		inject := []byte(harnessKitEmbedBootstrap)
		if idx := bytes.Index(rewritten, []byte("<head>")); idx >= 0 {
			at := idx + len("<head>")
			out := make([]byte, 0, len(rewritten)+len(inject))
			out = append(out, rewritten[:at]...)
			out = append(out, inject...)
			out = append(out, rewritten[at:]...)
			rewritten = out
		} else if idx := bytes.Index(rewritten, []byte("<head ")); idx >= 0 {
			// <head lang=...> etc.
			end := bytes.IndexByte(rewritten[idx:], '>')
			if end >= 0 {
				at := idx + end + 1
				out := make([]byte, 0, len(rewritten)+len(inject))
				out = append(out, rewritten[:at]...)
				out = append(out, inject...)
				out = append(out, rewritten[at:]...)
				rewritten = out
			}
		}
	}

	resp.Body = io.NopCloser(bytes.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	// Prevent intermediaries from serving a pre-rewrite cached shell.
	resp.Header.Set("Cache-Control", "no-store")
	return nil
}

func requestCorrelationID(r *http.Request) string {
	if existing := strings.TrimSpace(r.Header.Get("X-Correlation-ID")); existing != "" && len(existing) <= 128 {
		valid := true
		for _, ch := range existing {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.') {
				valid = false
				break
			}
		}
		if valid {
			return existing
		}
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err == nil {
		return hex.EncodeToString(raw)
	}
	return "unavailable"
}

type errorResponse struct {
	Error         string                       `json:"error"`
	Message       string                       `json:"message"`
	CorrelationID string                       `json:"correlationId"`
	HarnessKit    *supervisor.HarnessKitStatus `json:"harnesskit,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, message, correlationID string, hkStatus *supervisor.HarnessKitStatus) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error:         code,
		Message:       message,
		CorrelationID: correlationID,
		HarnessKit:    hkStatus,
	})
}
