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
	if !strings.Contains(body, "toCleanPath") {
		t.Fatal("expected clean-path helper for SPA routing (Remotion)")
	}
	if !strings.Contains(body, "toProxiedNetwork") {
		t.Fatal("expected network rewrite helper in injected script")
	}
	if !strings.Contains(body, "originalReplaceState") {
		t.Fatal("expected history.replaceState clean-path bootstrap")
	}
	// Injected target URL must appear (not the placeholder)
	if strings.Contains(body, "__TARGET_URL__") {
		t.Fatal("expected __TARGET_URL__ placeholder to be replaced")
	}
	if !strings.Contains(body, upstream.URL) {
		t.Fatal("expected upstream URL embedded in inject script")
	}
	// Bootstrap is prepended so it runs before any app script (Remotion boot).
	if !strings.HasPrefix(strings.TrimSpace(body), `<base href="`) {
		t.Fatalf("expected <base> prepended at document start, got: %s", snippetAround(body, "", 80))
	}
	if idxBase, idxHead := strings.Index(body, `<base href="`), strings.Index(body, `<head`); idxHead >= 0 && idxBase > idxHead {
		t.Fatal("expected <base>/bootstrap before original <head>")
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

	// Prefer real server so Flusher is available on the proxy side too.
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
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS * on streamed SSE")
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
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("response body = %q", rec.Body.String())
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

// Regression: browser sends Accept-Encoding: gzip; if we forward it, Go will
// not decompress, we strip Content-Encoding, and the client gets binary JSON.
func TestHandleProxy_DecompressesGzipJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Browser-style Accept-Encoding must not be forwarded (breaks decompress).
		if ae := r.Header.Get("Accept-Encoding"); strings.Contains(ae, "br") || strings.Contains(ae, "zstd") {
			t.Errorf("proxy must not forward browser Accept-Encoding, got %q", ae)
		}
		// Origin should be rewritten to the upstream target, not :38080.
		if !strings.HasPrefix(r.Header.Get("Origin"), "http://127.0.0.1:") {
			t.Errorf("expected Origin rewritten to upstream, got %q", r.Header.Get("Origin"))
		}
		w.Header().Set("Content-Type", "application/json")
		// Manual gzip like a typical Node/Express static compressor.
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(`{"version":"1.2.3"}`))
		_ = gz.Close()
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/proxy?url="+upstream.URL+"/api/version", nil)
	// Mimic Chrome
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Origin", "http://localhost:38080")
	req.Header.Set("Referer", "http://localhost:38080/api/proxy?url="+url.QueryEscape(upstream.URL+"/"))
	rec := httptest.NewRecorder()
	handleProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "" {
		t.Fatalf("client must not see Content-Encoding after decompress, got %q", ce)
	}
	body := rec.Body.String()
	if body != `{"version":"1.2.3"}` {
		t.Fatalf("expected decompressed JSON, got %q (len=%d)", body, len(body))
	}
	if !strings.Contains(proxyInjectScript, "originalFetch.call") {
		t.Fatal("fetch rewrite must use .call(this, input, init) not apply(arguments)")
	}
}

func snippetAround(s, needle string, n int) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return s
	}
	end := i + n
	if end > len(s) {
		end = len(s)
	}
	return s[i:end]
}
