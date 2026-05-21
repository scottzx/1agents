package gateway

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
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
