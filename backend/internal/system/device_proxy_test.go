package system

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestParseProxyPath(t *testing.T) {
	cases := []struct {
		in       string
		wantDev  string
		wantRest string
		wantOK   bool
	}{
		{"/api/proxy/mac/api/projects", "mac", "/api/projects", true},
		{"/api/proxy/mac/ws", "mac", "/ws", true},
		{"/api/proxy/mac", "mac", "/", true},
		{"/api/proxy/mac/", "mac", "/", true},
		{"/api/proxy/", "", "", false},
		{"/api/proxy", "", "", false},
		{"/other/mac/x", "", "", false},
	}
	for _, c := range cases {
		dev, rest, ok := parseProxyPath(c.in)
		if dev != c.wantDev || rest != c.wantRest || ok != c.wantOK {
			t.Errorf("parseProxyPath(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, dev, rest, ok, c.wantDev, c.wantRest, c.wantOK)
		}
	}
}

func TestNormalizeAddr(t *testing.T) {
	if got := normalizeAddr("100.1.1.1"); got != "100.1.1.1:"+oneAgentsPort {
		t.Errorf("bare IP: got %q", got)
	}
	if got := normalizeAddr("100.1.1.1:9000"); got != "100.1.1.1:9000" {
		t.Errorf("IP:port preserved: got %q", got)
	}
}

func TestResolveTarget(t *testing.T) {
	s := newTestStore(t, time.Unix(1_700_000_000, 0))
	if _, err := s.upsert(Device{ID: "mac", Address: "100.1.1.1:38080"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.upsert(Device{ID: "lin", TailscaleIP: "100.2.2.2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.upsert(Device{ID: "noaddr"}); err != nil {
		t.Fatal(err)
	}

	if addr, code := s.resolveTarget("mac"); addr != "100.1.1.1:38080" || code != "" {
		t.Errorf("mac: got (%q,%q)", addr, code)
	}
	if addr, code := s.resolveTarget("lin"); addr != "100.2.2.2:"+oneAgentsPort || code != "" {
		t.Errorf("lin (tailscale fallback): got (%q,%q)", addr, code)
	}
	if _, code := s.resolveTarget("noaddr"); code != errDeviceOffline {
		t.Errorf("noaddr: want device_offline, got %q", code)
	}
	if _, code := s.resolveTarget("ghost"); code != errDeviceUnknown {
		t.Errorf("ghost: want device_unknown, got %q", code)
	}
}

// proxyToStore swaps defaultDeviceStore for a test store by writing to the
// path the handler reads. The handler calls defaultDeviceStore() internally
// (honoring ONEAGENTS_HOME), so we point that env at a temp dir and seed it.
func seedDeviceFile(t *testing.T, devices []Device) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	s := defaultDeviceStore()
	for _, d := range devices {
		if _, err := s.upsert(d); err != nil {
			t.Fatalf("seed upsert: %v", err)
		}
	}
}

func TestDeviceProxyHandlerUnknownDevice(t *testing.T) {
	seedDeviceFile(t, nil)
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/ghost/api/projects", nil)
	rec := httptest.NewRecorder()
	h.DeviceProxyHandler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != string(errDeviceUnknown) {
		t.Errorf("error = %q, want %q", body["error"], errDeviceUnknown)
	}
}

func TestDeviceProxyHandlerHTTPForward(t *testing.T) {
	// Upstream target device echoes the path + a header so we can assert
	// path-rewrite and header pass-through.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo-Path", r.URL.Path)
		w.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "hello from device")
	}))
	defer upstream.Close()

	addr := strings.TrimPrefix(upstream.URL, "http://")
	seedDeviceFile(t, []Device{{ID: "mac", Address: addr}})

	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/mac/api/projects", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.DeviceProxyHandler(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (verbatim from upstream)", rec.Code)
	}
	if got := rec.Header().Get("X-Echo-Path"); got != "/api/projects" {
		t.Errorf("upstream saw path %q, want /api/projects (prefix stripped)", got)
	}
	if got := rec.Header().Get("X-Echo-Auth"); got != "Bearer secret" {
		t.Errorf("Authorization not forwarded: %q", got)
	}
	if rec.Body.String() != "hello from device" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestDeviceProxyHandlerHTTPDialFailure(t *testing.T) {
	// Point at a closed port → direct dial fails → typed proxy_timeout error.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close() // close so nothing is listening

	seedDeviceFile(t, []Device{{ID: "mac", Address: addr}})

	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/mac/api/projects", nil)
	rec := httptest.NewRecorder()
	h.DeviceProxyHandler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != string(errProxyTimeout) {
		t.Errorf("error = %q, want %q", body["error"], errProxyTimeout)
	}
}

func TestDeviceProxyHandlerWebSocketTunnel(t *testing.T) {
	// Upstream device WS server echoes each frame with an "echo:" prefix.
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			_ = c.WriteMessage(mt, append([]byte("echo:"), msg...))
		}
	}))
	defer upstream.Close()

	addr := strings.TrimPrefix(upstream.URL, "http://")
	seedDeviceFile(t, []Device{{ID: "mac", Address: addr}})

	// Front the handler with an httptest server so we get a real WS handshake.
	h := &Handler{}
	proxySrv := httptest.NewServer(http.HandlerFunc(h.DeviceProxyHandler))
	defer proxySrv.Close()

	wsURL := url.URL{Scheme: "ws", Host: strings.TrimPrefix(proxySrv.URL, "http://"), Path: "/api/proxy/mac/ws"}
	client, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		t.Fatalf("dial proxy ws: %v (resp=%v)", err, resp)
	}
	defer client.Close()

	if err := client.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(msg) != "echo:ping" {
		t.Errorf("got %q, want echo:ping", msg)
	}
}
