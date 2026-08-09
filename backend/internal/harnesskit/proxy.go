package harnesskit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
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
var browserRoutes = map[routePermission]struct{}{
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
	{http.MethodPost, "/api/list_kit_asset_candidates"}:{},
	{http.MethodPost, "/api/get_extension_content"}:    {},
	{http.MethodPost, "/api/get_all_tags"}:             {},
	{http.MethodPost, "/api/get_all_packs"}:            {},
	{http.MethodPost, "/api/list_agent_configs"}:       {},
	{http.MethodPost, "/api/list_hermes_categories"}:   {},
	{http.MethodPost, "/api/get_cli_with_children"}:    {},
	{http.MethodPost, "/api/get_skill_locations"}:      {},
	{http.MethodPost, "/api/read_config_file_preview"}: {},
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

	upstreamPath := "/api" + strings.TrimPrefix(r.URL.Path, proxyPrefix)
	if _, ok := browserRoutes[routePermission{method: r.Method, path: upstreamPath}]; !ok {
		writeError(w, http.StatusForbidden, "route_not_allowed", "HarnessKit route is not available through the embedded boundary", correlationID, nil)
		return
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
