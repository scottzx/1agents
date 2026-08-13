package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseAndEncodeWebProxyPath_RemotionComposition(t *testing.T) {
	target := "http://localhost:3000/TalkingHeadComposition"
	path, err := encodeWebProxyPath(target)
	if err != nil {
		t.Fatal(err)
	}
	// Last path segment must be the composition id (Remotion deriveCanvasContentFromRoute).
	if !strings.HasSuffix(path, "/TalkingHeadComposition") {
		t.Fatalf("path should end with composition id, got %q", path)
	}
	if !strings.HasPrefix(path, "/api/webproxy/") {
		t.Fatalf("path prefix: %q", path)
	}
	// Must NOT be the old query form that makes pathname == /api/proxy
	if strings.Contains(path, "?url=") {
		t.Fatal("must use path form, not ?url=")
	}

	u, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseWebProxyPath(u)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("round-trip: got %q want %q", got, target)
	}

	// Remotion: last segment after split
	parts := strings.Split(strings.Trim(path, "/"), "/")
	last := parts[len(parts)-1]
	if last != "TalkingHeadComposition" {
		t.Fatalf("Remotion last segment = %q, want TalkingHeadComposition (full path %q)", last, path)
	}
}

func TestHandleWebProxy_InjectsCleanPathForComposition(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/TalkingHeadComposition" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><head><title>studio</title></head><body>remotion</body></html>`)
	}))
	defer upstream.Close()

	// Build webproxy path as if target were upstream + composition
	origin := upstream.URL // http://127.0.0.1:port
	target := origin + "/TalkingHeadComposition"
	proxyPath, err := encodeWebProxyPath(target)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, proxyPath, nil)
	rec := httptest.NewRecorder()
	handleWebProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "TalkingHeadComposition") {
		t.Fatal("expected composition path embedded in inject/bootstrap")
	}
	if !strings.Contains(body, "toHistoryURL") {
		t.Fatal("expected history helper that keeps webproxy path (reload-safe)")
	}
	if !strings.Contains(body, "buildWebProxyURL") {
		t.Fatal("expected webproxy network rewrite helper")
	}
	if !strings.Contains(body, "wrapWorkerCtor") {
		t.Fatal("expected Worker rewrite for cross-origin base scripts")
	}
	if !strings.Contains(body, "proxyMediaUrl") {
		t.Fatal("expected Image/media src rewrite for WebGL/Phaser texImage2D")
	}
	if !strings.Contains(body, "wrapSrcAccessor") {
		t.Fatal("expected HTMLImageElement.src accessor wrap")
	}
	if !strings.Contains(body, "HTMLImageElement") {
		t.Fatal("expected HTMLImageElement hook in bootstrap")
	}
	// Must NOT rewrite history to bare /TalkingHeadComposition (404 on reload)
	if strings.Contains(body, "toCleanPath") {
		t.Fatal("toCleanPath should be removed — bare paths 404 on 1agents host")
	}
	if !strings.HasPrefix(strings.TrimSpace(body), `<base href="`) {
		t.Fatal("expected <base> prepended")
	}
	// base should be origin root, not full composition URL
	if strings.Contains(body, `<base href="`+target+`"`) {
		t.Fatal("base href must be origin/, not full composition URL")
	}
}

func TestHandleWebProxy_InjectsMediaSrcRewriteForWebGL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><body><img src="/sprite.png"></body></html>`)
	}))
	defer upstream.Close()

	proxyPath, err := encodeWebProxyPath(upstream.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, proxyPath, nil)
	rec := httptest.NewRecorder()
	handleWebProxy(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"proxyMediaUrl",
		"wrapSrcAccessor",
		"wrapMediaSetAttribute",
		"scanMediaTree",
		"HTMLImageElement",
		"HTMLMediaElement",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("bootstrap missing %q (needed so Phaser texImage2D is same-origin)", needle)
		}
	}
	// Must not blindly rewrite data:/blob: (would break inline textures).
	if !strings.Contains(body, `resolved.protocol !== 'http:'`) {
		t.Fatal("expected http(s)-only media rewrite guard")
	}
}

func TestHandleProxy_InjectsEventSourceRewrite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Frame-Options", "DENY")
		_, _ = io.WriteString(w, `<!doctype html><html><head lang="en"><title>t</title></head><body>ok</body></html>`)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/proxy?url="+upstream.URL+"/", nil)
	rec := httptest.NewRecorder()
	handleProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Frame-Options") != "" {
		t.Fatal("expected X-Frame-Options to be stripped")
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<base href="`) {
		t.Fatal("expected <base> injection")
	}
	if !strings.Contains(body, "EventSource") {
		t.Fatal("expected EventSource rewrite in injected script")
	}
	if !strings.Contains(body, "buildWebProxyURL") {
		t.Fatal("expected webproxy helper for SPA routing (Remotion)")
	}
	if strings.Contains(body, "__TARGET_URL__") {
		t.Fatal("expected __TARGET_URL__ placeholder to be replaced")
	}
}

func TestHandleProxy_StreamsSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream ResponseWriter is not a Flusher")
		}
		_, _ = io.WriteString(w, "data: hello\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: world\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	proxy := httptest.NewServer(http.HandlerFunc(handleProxy))
	defer proxy.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(proxy.URL + "/api/proxy?url=" + upstream.URL + "/events")
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "data: hello") || !strings.Contains(got, "data: world") {
		t.Fatalf("unexpected SSE body: %q", got)
	}
}

func TestHandleProxy_ForwardsPOSTBody(t *testing.T) {
	var gotMethod, gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/proxy?url="+upstream.URL+"/api", strings.NewReader(`{"x":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("upstream method = %q", gotMethod)
	}
	if gotBody != `{"x":1}` {
		t.Fatalf("upstream body = %q", gotBody)
	}
}

func TestHandleProxy_MissingURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/proxy", nil)
	rec := httptest.NewRecorder()
	handleProxy(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandleProxy_DecompressesGzipJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ae := r.Header.Get("Accept-Encoding"); strings.Contains(ae, "br") || strings.Contains(ae, "zstd") {
			t.Errorf("proxy must not forward browser Accept-Encoding, got %q", ae)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(`{"version":"1.2.3"}`))
		_ = gz.Close()
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/proxy?url="+upstream.URL+"/api/version", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Origin", "http://localhost:38080")
	rec := httptest.NewRecorder()
	handleProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "" {
		t.Fatalf("client must not see Content-Encoding after decompress, got %q", ce)
	}
	if rec.Body.String() != `{"version":"1.2.3"}` {
		t.Fatalf("expected decompressed JSON, got %q", rec.Body.String())
	}
}
