package gateway

import (
	"fmt"
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

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
	}

	// Log proxy errors without crashing the server.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[gateway] proxy error for %s: %v", r.URL.Path, err)
		http.Error(w, "Terminal service unavailable. Please wait a moment and refresh.", http.StatusBadGateway)
	}

	return proxy
}

// NewCCConnectProxy creates an http.Handler that transparently reverse-proxies
// requests to the locally-running CC-Connect management server on mgmtPort.
func NewCCConnectProxy(mgmtPort int) http.Handler {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", mgmtPort),
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		// Strip /cc-connect prefix so it hits the correct endpoints in CC-Connect
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/cc-connect")
		if req.URL.RawPath != "" {
			req.URL.RawPath = strings.TrimPrefix(req.URL.RawPath, "/cc-connect")
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[gateway] cc-connect proxy error for %s: %v", r.URL.Path, err)
		http.Error(w, "CC-Connect service unavailable.", http.StatusBadGateway)
	}

	return proxy
}

// NewBridgeProxy creates an http.Handler that transparently reverse-proxies
// WebSocket and HTTP requests to the locally-running CC-Connect bridge server
// on bridgePort (used by the frontend to establish a direct bridge connection).
func NewBridgeProxy(bridgePort int) http.Handler {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", bridgePort),
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	originalDirector := proxy.Director

	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[gateway] bridge proxy error for %s: %v", r.URL.Path, err)
		http.Error(w, "Bridge service unavailable.", http.StatusBadGateway)
	}

	return proxy
}
