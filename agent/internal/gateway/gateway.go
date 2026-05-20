package gateway

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewTtydProxy creates an http.Handler that transparently reverse-proxies
// both plain HTTP and WebSocket connections to the locally-running ttyd process.
//
// ttydAddr should be in "host:port" form, e.g. "127.0.0.1:7681".
func NewTtydProxy(ttydAddr string) http.Handler {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   ttydAddr,
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Wrap the default Director so we can fix up headers that are required
	// for a successful WebSocket upgrade through a reverse proxy.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host

		// httputil.ReverseProxy strips the Upgrade header by default.
		// For WebSocket connections we must explicitly re-add it.
		if isWebSocket(req) {
			req.URL.Scheme = "ws"
		}
	}

	// Log proxy errors without crashing the server.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[gateway] proxy error for %s: %v", r.URL.Path, err)
		http.Error(w, "Terminal service unavailable. Please wait a moment and refresh.", http.StatusBadGateway)
	}

	return proxy
}

// isWebSocket returns true when the request contains an HTTP Upgrade header
// with value "websocket" (case-insensitive, as required by RFC 6455).
func isWebSocket(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
