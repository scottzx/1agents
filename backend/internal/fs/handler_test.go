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
	htmlContent := "<html><body><h1>Hello 1agents</h1></body></html>"
	testFile := "page.html"
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

	t.Run("Serve index.html successfully via subpath /api/fs/view/index.html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/view/"+testFile, nil)
		w := httptest.NewRecorder()

		h.View(w, req)

		res := w.Result()
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", res.StatusCode)
		}

		contentType := res.Header.Get("Content-Type")
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

	t.Run("Serve index.html when requesting directory containing it", func(t *testing.T) {
		// Create a subdirectory with index.html
		subDir := filepath.Join(tempDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}
		subIndexContent := "<html>Sub Index</html>"
		if err := os.WriteFile(filepath.Join(subDir, "index.html"), []byte(subIndexContent), 0644); err != nil {
			t.Fatalf("failed to write sub index file: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/fs/view/subdir/", nil)
		w := httptest.NewRecorder()

		h.View(w, req)

		res := w.Result()
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", res.StatusCode)
		}

		bodyBytes := w.Body.Bytes()
		if string(bodyBytes) != subIndexContent {
			t.Errorf("expected body %q, got %q", subIndexContent, string(bodyBytes))
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

	t.Run("Serve index.html when path has collapsed leading slash and is inside registered workspace", func(t *testing.T) {
		// Mock home directory by setting HOME/USERPROFILE env vars
		origHome := os.Getenv("HOME")
		origUserProfile := os.Getenv("USERPROFILE")

		mockHome := tempDir
		os.Setenv("HOME", mockHome)
		os.Setenv("USERPROFILE", mockHome)
		defer func() {
			os.Setenv("HOME", origHome)
			os.Setenv("USERPROFILE", origUserProfile)
		}()

		// Create the workspaces directory and workspaces_dir.json
		wsDir := filepath.Join(mockHome, ".1agents")
		if err := os.MkdirAll(wsDir, 0755); err != nil {
			t.Fatalf("failed to create mock .1agents dir: %v", err)
		}

		// Registered workspaces configuration
		// Let's register tempDir (which is our workspace root)
		wsCfgContent := `{"workspaces": [{"path": "` + filepath.ToSlash(tempDir) + `"}]}`
		if err := os.WriteFile(filepath.Join(wsDir, "workspaces_dir.json"), []byte(wsCfgContent), 0644); err != nil {
			t.Fatalf("failed to write mock workspaces_dir.json: %v", err)
		}

		// Create a test file inside tempDir
		nestedFile := "nested/page.html"
		absNestedFile := filepath.Join(tempDir, nestedFile)
		if err := os.MkdirAll(filepath.Dir(absNestedFile), 0755); err != nil {
			t.Fatalf("failed to create nested dir: %v", err)
		}
		nestedContent := "<html>Nested page</html>"
		if err := os.WriteFile(absNestedFile, []byte(nestedContent), 0644); err != nil {
			t.Fatalf("failed to write nested test file: %v", err)
		}

		// Now make a request with absolute path but COLLAPSED leading slash
		// Absolute path is tempDir + "/" + nestedFile
		// E.g., /private/var/.../nested/page.html -> private/var/.../nested/page.html
		absPathCleaned := filepath.Clean(absNestedFile)
		collapsedPath := absPathCleaned
		if len(collapsedPath) > 0 && (collapsedPath[0] == '/' || collapsedPath[0] == '\\') {
			collapsedPath = collapsedPath[1:]
		}

		req := httptest.NewRequest(http.MethodGet, "/api/fs/view/"+collapsedPath, nil)
		w := httptest.NewRecorder()

		h.View(w, req)

		res := w.Result()
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", res.StatusCode, w.Body.String())
		}

		bodyBytes := w.Body.Bytes()
		if string(bodyBytes) != nestedContent {
			t.Errorf("expected body %q, got %q", nestedContent, string(bodyBytes))
		}
	})
}
