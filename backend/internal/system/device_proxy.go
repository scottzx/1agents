package system

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// device proxy routing layer (issue #111, Phase 3 of the multi-device mesh #115).
//
// The local 1agents host is the single proxy entry point: the frontend always
// talks to its own host, and the host routes a request to the target device.
//
//	GET|POST|PUT|DELETE /api/proxy/{deviceId}/{path...}  → HTTP reverse proxy
//	GET                 /api/proxy/{deviceId}/ws         → WebSocket tunnel
//
// Routing strategy: prefer a Tailscale/LAN direct connection (low latency, same
// trust model as discovery in #110). The spec also calls for a Happy-server
// relay fallback, but the relay RPC stack (E2E crypto + callMachine) lives
// entirely in the frontend relayClient.ts — there is no Go relay client. Rather
// than port that whole stack here, the fallback is a clear, typed error so the
// frontend can decide to retry over its own relay path. This keeps the backend
// minimal and avoids inventing a second tunnel protocol.

// directDialTimeout bounds the initial connection to the target device on the
// direct path. The spec (#111) specifies ~1s so a dead peer fails fast and the
// caller can fall back.
const directDialTimeout = time.Second

// proxyErrorCode is the typed error contract shared with the frontend.
type proxyErrorCode string

const (
	errDeviceUnknown proxyErrorCode = "device_unknown" // deviceId not in registry (anti-spoof)
	errDeviceOffline proxyErrorCode = "device_offline" // device known but has no reachable address
	errProxyTimeout  proxyErrorCode = "proxy_timeout"  // direct connection attempt failed/timed out
)

// writeProxyError emits the unified {error, message} JSON shape.
func writeProxyError(w http.ResponseWriter, status int, code proxyErrorCode, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   string(code),
		"message": msg,
	})
}

// parseProxyPath splits /api/proxy/{deviceId}/{rest} into (deviceId, "/rest").
// rest always begins with "/" (or is "/" when no sub-path was given). ok is
// false when the path is malformed (no deviceId segment).
func parseProxyPath(p string) (deviceID, rest string, ok bool) {
	const prefix = "/api/proxy/"
	if !strings.HasPrefix(p, prefix) {
		return "", "", false
	}
	tail := strings.TrimPrefix(p, prefix)
	if tail == "" {
		return "", "", false
	}
	if i := strings.IndexByte(tail, '/'); i >= 0 {
		deviceID = tail[:i]
		rest = tail[i:]
	} else {
		deviceID = tail
		rest = "/"
	}
	if deviceID == "" {
		return "", "", false
	}
	return deviceID, rest, true
}

// resolveTarget looks up a device by id in the local registry and returns its
// reachable "host:port" address. It enforces the anti-spoof rule (#111): the
// deviceId must exist in the registry. Returns a typed error code otherwise.
func (s *deviceStore) resolveTarget(deviceID string) (addr string, code proxyErrorCode) {
	for _, d := range s.list() {
		if d.ID != deviceID {
			continue
		}
		// Prefer the explicit Address, fall back to the tailnet IP.
		if d.Address != "" {
			return normalizeAddr(d.Address), ""
		}
		if d.TailscaleIP != "" {
			return normalizeAddr(d.TailscaleIP), ""
		}
		return "", errDeviceOffline
	}
	return "", errDeviceUnknown
}

// normalizeAddr ensures the address carries a port, defaulting to the standard
// 1agents port when only an IP/host was stored.
func normalizeAddr(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, oneAgentsPort)
}

// DeviceProxyHandler handles /api/proxy/{deviceId}/... for both HTTP and
// WebSocket. It is the single entry point registered in the router.
func (h *Handler) DeviceProxyHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, rest, ok := parseProxyPath(r.URL.Path)
	if !ok {
		writeProxyError(w, http.StatusBadRequest, errDeviceUnknown, "missing device id in proxy path")
		return
	}

	store := defaultDeviceStore()
	addr, code := store.resolveTarget(deviceID)
	if code != "" {
		status := http.StatusBadGateway
		msg := "target device is not reachable"
		switch code {
		case errDeviceUnknown:
			status = http.StatusForbidden
			msg = "device is not registered for this account"
		case errDeviceOffline:
			status = http.StatusBadGateway
			msg = "device has no known address; refresh device discovery"
		}
		writeProxyError(w, status, code, msg)
		return
	}

	// A request is a WebSocket upgrade when the client asks for it. terminal
	// (xterm.js) connects to /api/proxy/{deviceId}/ws.
	if isWebSocketUpgrade(r) {
		proxyWebSocket(w, r, addr, rest)
		return
	}
	proxyHTTP(w, r, addr, rest)
}

// isWebSocketUpgrade reports whether the request is a WebSocket handshake.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// proxyHTTP reverse-proxies a plain HTTP request to the target device, rewriting
// the path to drop the /api/proxy/{deviceId} prefix. Request headers (incl.
// Content-Type / Authorization) and the body pass through; the target's status
// code and body are returned verbatim. A failed direct dial maps to the typed
// proxy_timeout contract.
func proxyHTTP(w http.ResponseWriter, r *http.Request, addr, rest string) {
	target := &url.URL{Scheme: "http", Host: addr}
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Fail fast on the direct path so the caller can fall back to its own relay.
	proxy.Transport = &http.Transport{
		DialContext:           (&net.Dialer{Timeout: directDialTimeout}).DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		req.Host = target.Host
		req.URL.Path = rest
		req.URL.RawPath = ""
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[device-proxy] direct dial to %s failed: %v", addr, err)
		// Direct path failed → the relay fallback would kick in here, but the Go
		// backend has no relay client (see file header). Surface a typed error.
		writeProxyError(w, http.StatusBadGateway, errProxyTimeout,
			"direct connection to device failed: "+err.Error())
	}

	proxy.ServeHTTP(w, r)
}

// wsDialer dials the upstream device with the same fast-fail timeout as HTTP.
var wsDialer = &websocket.Dialer{
	HandshakeTimeout: directDialTimeout,
}

// wsUpgrader upgrades the incoming client connection. Origin checks are handled
// by the surrounding auth middleware / same-origin gateway, so allow all here.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// proxyWebSocket establishes a WS connection to the target device and pumps
// frames in both directions until either side closes. Used by the terminal.
func proxyWebSocket(w http.ResponseWriter, r *http.Request, addr, rest string) {
	upstreamURL := url.URL{Scheme: "ws", Host: addr, Path: rest, RawQuery: r.URL.RawQuery}

	// Forward a curated set of request headers (subprotocols, auth) to upstream.
	// gorilla rejects the hop-by-hop Upgrade/Connection headers, so omit them.
	fwd := http.Header{}
	if v := r.Header.Get("Authorization"); v != "" {
		fwd.Set("Authorization", v)
	}
	if v := r.Header.Get("Sec-WebSocket-Protocol"); v != "" {
		fwd.Set("Sec-WebSocket-Protocol", v)
	}
	if v := r.Header.Get("Cookie"); v != "" {
		fwd.Set("Cookie", v)
	}

	upstream, resp, err := wsDialer.Dial(upstreamURL.String(), fwd)
	if err != nil {
		log.Printf("[device-proxy] ws dial to %s failed: %v", upstreamURL.String(), err)
		writeProxyError(w, http.StatusBadGateway, errProxyTimeout,
			"websocket connection to device failed: "+err.Error())
		return
	}
	defer upstream.Close()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	// Carry over the negotiated subprotocol so the client handshake matches.
	var respHeader http.Header
	if proto := upstream.Subprotocol(); proto != "" {
		respHeader = http.Header{"Sec-WebSocket-Protocol": {proto}}
	}

	client, err := wsUpgrader.Upgrade(w, r, respHeader)
	if err != nil {
		// Upgrade already wrote an error response.
		log.Printf("[device-proxy] client ws upgrade failed: %v", err)
		return
	}
	defer client.Close()

	pumpWebSocket(client, upstream)
}

// pumpWebSocket relays raw frames between two WS connections. It returns once
// either direction errors (close, network drop), then both deferred closes run
// so the peer is notified — the frontend treats a close as "show reconnect".
func pumpWebSocket(a, b *websocket.Conn) {
	done := make(chan struct{}, 2)
	copyFrames := func(dst, src *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := src.ReadMessage()
			if err != nil {
				return
			}
			if err := dst.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}
	go copyFrames(a, b)
	go copyFrames(b, a)
	<-done // first side to error tears down the tunnel via the deferred Closes
}
