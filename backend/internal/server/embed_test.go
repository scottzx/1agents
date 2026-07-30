package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbedBundleCandidates_prefersStaticEmbed(t *testing.T) {
	static := filepath.Join(string(filepath.Separator), "opt", "web", "dist")
	got := embedBundleCandidates(static, "harnesskit-embed.js", []string{
		"modules/HarnessKit/dist-embed/harnesskit-embed.js",
	})
	if len(got) < 2 {
		t.Fatalf("expected at least static + monorepo candidates, got %v", got)
	}
	if got[0] != filepath.Join(static, "embed", "harnesskit-embed.js") {
		t.Fatalf("first candidate should be StaticDir/embed/file, got %q", got[0])
	}
	if got[1] != filepath.Join(static, "harnesskit-embed.js") {
		t.Fatalf("second candidate should be StaticDir/file, got %q", got[1])
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "modules/HarnessKit/dist-embed/harnesskit-embed.js") {
		t.Fatalf("missing monorepo fallback: %v", got)
	}
}

func TestServeHarnessKitEmbedPageSanitizesAttributes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/extensions/?theme=%22%3Ebad&lang=%3Cscript%3E", nil)
	rec := httptest.NewRecorder()
	serveHarnessKitEmbedPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `theme="system" language="en"`) {
		t.Fatalf("unexpected sanitized attributes: %s", body)
	}
	if strings.Contains(body, "<script>") || strings.Contains(body, `">bad`) {
		t.Fatalf("untrusted query reached page: %s", body)
	}
	if !strings.Contains(body, "/api/embed/harnesskit-embed.js") {
		t.Fatalf("HarnessKit embed script missing: %s", body)
	}
}

func TestEmbedBundleCandidates_emptyStaticStillHasMonorepo(t *testing.T) {
	got := embedBundleCandidates("", "cc-connect-embed.js", []string{
		"modules/cc-connect/web/dist-embed/cc-connect-embed.js",
	})
	found := false
	for _, p := range got {
		if p == "modules/cc-connect/web/dist-embed/cc-connect-embed.js" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected monorepo path in %v", got)
	}
}
