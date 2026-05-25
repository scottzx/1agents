package fs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandler_View(t *testing.T) {
	// Create a temporary sandbox directory
	tempDir, err := os.MkdirTemp("", "fs-test-sandbox-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test HTML file
	htmlContent := "<html><body><h1>Hello remote-agents</h1></body></html>"
	testFile := "index.html"
	absTestFile := filepath.Join(tempDir, testFile)
	if err := os.WriteFile(absTestFile, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Initialize the handler
	h := NewHandler(tempDir)

	t.Run("Serve index.html successfully with correct Content-Type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/view?path="+testFile, nil)
		w := httptest.NewRecorder()

		h.View(w, req)

		res := w.Result()
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", res.StatusCode)
		}

		contentType := res.Header.Get("Content-Type")
		if contentType == "" {
			t.Error("expected Content-Type header, got empty")
		}
		// Content-Type might be "text/html" or "text/html; charset=utf-8"
		if contentType != "text/html" && contentType != "text/html; charset=utf-8" {
			t.Errorf("expected text/html content type, got %s", contentType)
		}

		bodyBytes := w.Body.Bytes()
		if string(bodyBytes) != htmlContent {
			t.Errorf("expected body %q, got %q", htmlContent, string(bodyBytes))
		}
	})

	t.Run("Reject Directory Request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/view?path=.", nil)
		w := httptest.NewRecorder()

		h.View(w, req)

		res := w.Result()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400 (Bad Request) for directories, got %d", res.StatusCode)
		}
	})

	t.Run("Block Path Traversal Attempt", func(t *testing.T) {
		// Attempt to access parent directory or outside sandbox
		req := httptest.NewRequest(http.MethodGet, "/api/fs/view?path=../../etc/passwd", nil)
		w := httptest.NewRecorder()

		h.View(w, req)

		res := w.Result()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403 (Forbidden) for path traversal, got %d", res.StatusCode)
		}
	})
}
