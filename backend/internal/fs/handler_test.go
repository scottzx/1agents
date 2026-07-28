package fs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestHandler_ListCachesRecursiveTreePerProject(t *testing.T) {
	projectA := t.TempDir()
	projectB := t.TempDir()

	if err := os.MkdirAll(filepath.Join(projectA, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectA, "src", "nested", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectA, "node_modules", "dependency"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectB, "README.md"), []byte("project b"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(projectA)
	list := func(refresh bool) []FileEntry {
		t.Helper()
		url := "/api/fs/list?path=."
		if refresh {
			url += "&refresh=true"
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		h.List(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list returned %d: %s", w.Code, w.Body.String())
		}
		var entries []FileEntry
		if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
			t.Fatal(err)
		}
		return entries
	}

	entriesA := list(false)
	if len(entriesA) != 1 || entriesA[0].Name != "src" || len(entriesA[0].Children) != 1 || len(entriesA[0].Children[0].Children) != 1 {
		t.Fatalf("expected complete recursive tree, got %#v", entriesA)
	}
	firstCachedAt := h.treeCache[projectA].cachedAt

	if err := os.WriteFile(filepath.Join(projectA, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := list(false); len(got) != 1 {
		t.Fatalf("unexpired cache should hide external changes, got %#v", got)
	}

	time.Sleep(time.Millisecond)
	if got := list(true); len(got) != 2 {
		t.Fatalf("manual refresh should rebuild the tree, got %#v", got)
	}
	if !h.treeCache[projectA].cachedAt.After(firstCachedAt) {
		t.Fatal("manual refresh should reset the cache timestamp")
	}

	if err := os.WriteFile(filepath.Join(projectA, "expired.txt"), []byte("expired"), 0o644); err != nil {
		t.Fatal(err)
	}
	cached := h.treeCache[projectA]
	cached.cachedAt = time.Now().Add(-treeCacheTTL - time.Second)
	h.treeCache[projectA] = cached
	if got := list(false); len(got) != 3 {
		t.Fatalf("expired cache should rebuild automatically, got %#v", got)
	}

	if err := h.SetRoot(projectB); err != nil {
		t.Fatal(err)
	}
	entriesB := list(false)
	if len(entriesB) != 1 || entriesB[0].Name != "README.md" {
		t.Fatalf("expected independent project B tree, got %#v", entriesB)
	}
	if len(h.treeCache) != 2 {
		t.Fatalf("expected one cache per project, got %d", len(h.treeCache))
	}
}

func TestTreeCacheTTLIsFiveMinutes(t *testing.T) {
	if treeCacheTTL != 5*time.Minute {
		t.Fatalf("tree cache TTL = %s, want 5m", treeCacheTTL)
	}
}

func TestHandler_SearchReusesTreeCacheAndSharedIgnores(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Shared ignore set: both tree list and search must skip these.
	for _, ignored := range []string{"node_modules", ".vscode", "target", ".venv"} {
		dir := filepath.Join(root, ignored)
		if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "nested", "hidden.go"), []byte("package hidden"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler(root)
	search := func(query, tag string) []FileEntry {
		t.Helper()
		url := "/api/fs/search?query=" + query + "&tag=" + tag
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		h.Search(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("search returned %d: %s", w.Code, w.Body.String())
		}
		var entries []FileEntry
		if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
			t.Fatal(err)
		}
		return entries
	}

	// First search builds the tree cache.
	got := search("main", "all")
	if len(got) != 1 || got[0].Path != "src/main.go" {
		t.Fatalf("expected src/main.go, got %#v", got)
	}
	cachedAt := h.treeCache[root].cachedAt
	if cachedAt.IsZero() {
		t.Fatal("search should populate treeCache")
	}

	// External file must not appear while cache is warm (same TTL semantics as List).
	if err := os.WriteFile(filepath.Join(root, "extra.go"), []byte("package extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := search("extra", "all"); len(got) != 0 {
		t.Fatalf("warm cache should hide external files, got %#v", got)
	}

	// Second search must reuse the same cache entry (no rebuild).
	_ = search("readme", "doc")
	if !h.treeCache[root].cachedAt.Equal(cachedAt) {
		t.Fatal("second search should reuse treeCache without rebuilding")
	}

	// Tag filter: only docs from the cached tree.
	docs := search("", "doc")
	if len(docs) != 1 || docs[0].Name != "README.md" {
		t.Fatalf("expected only README.md for tag=doc, got %#v", docs)
	}

	// Ignored dirs must never surface, even with a matching name query.
	if hidden := search("hidden", "all"); len(hidden) != 0 {
		t.Fatalf("ignored dirs should be excluded from search, got %#v", hidden)
	}
}

func TestSearchTreeLimitAndMatch(t *testing.T) {
	tree := []FileEntry{
		{Name: "a.go", Path: "a.go", IsDir: false},
		{
			Name:  "pkg",
			Path:  "pkg",
			IsDir: true,
			Children: []FileEntry{
				{Name: "b.go", Path: "pkg/b.go", IsDir: false},
				{Name: "note.md", Path: "pkg/note.md", IsDir: false},
			},
		},
	}
	got := searchTree(tree, "pkg/", "all", 10)
	if len(got) != 2 {
		t.Fatalf("path substring should match nested files, got %#v", got)
	}
	got = searchTree(tree, "", "code", 1)
	if len(got) != 1 || got[0].Name != "a.go" {
		t.Fatalf("limit=1 should return first code file only, got %#v", got)
	}
}

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

		// Register tempDir as a workspace in meta.db (the unified registry that
		// checkWorkspaces now consults instead of workspaces_dir.json).
		t.Setenv("ONEAGENTS_HOME", mockHome)
		db, err := meta.OpenDefault()
		if err != nil {
			t.Fatalf("open meta: %v", err)
		}
		if err := db.EnsureWorkspaceProject(meta.Project{
			ID: "ws-test", Name: "test", WorkspacePath: tempDir,
		}); err != nil {
			t.Fatalf("register workspace: %v", err)
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

func TestHandler_ReadOnlyPreviewAllowsAnyAbsolutePathFromLoopback(t *testing.T) {
	root := t.TempDir()
	outsideRoot := t.TempDir()
	path := filepath.Join(outsideRoot, "设计 文档.md")
	content := []byte("# outside workspace")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(root)

	t.Run("read", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/read?path="+url.QueryEscape(path), nil)
		req.RemoteAddr = "127.0.0.1:43120"
		w := httptest.NewRecorder()

		h.Read(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("read returned %d: %s", w.Code, w.Body.String())
		}
		if got := w.Body.String(); got != string(content) {
			t.Fatalf("read body = %q, want %q", got, content)
		}
	})

	t.Run("view query preserves absolute path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/view?path="+url.QueryEscape(path), nil)
		req.RemoteAddr = "[::1]:43121"
		w := httptest.NewRecorder()

		h.View(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("view returned %d: %s", w.Code, w.Body.String())
		}
		if got := w.Body.String(); got != string(content) {
			t.Fatalf("view body = %q, want %q", got, content)
		}
	})
}

func TestHandler_AbsolutePathOutsideWorkspaceRemainsSandboxedForRemoteAndWrites(t *testing.T) {
	root := t.TempDir()
	outsideRoot := t.TempDir()
	path := filepath.Join(outsideRoot, "private.txt")
	if err := os.WriteFile(path, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Isolate the workspace registry so developer state cannot make the
	// outside temp directory an accepted workspace during this test.
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	h := NewHandler(root)

	t.Run("remote read", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/read?path="+url.QueryEscape(path), nil)
		req.RemoteAddr = "192.0.2.1:43122"
		w := httptest.NewRecorder()

		h.Read(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("remote read returned %d, want 403", w.Code)
		}
	})

	t.Run("loopback write", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/fs/write?path="+url.QueryEscape(path),
			strings.NewReader("changed"),
		)
		req.RemoteAddr = "127.0.0.1:43123"
		w := httptest.NewRecorder()

		h.Write(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("loopback write returned %d, want 403", w.Code)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "private" {
			t.Fatalf("outside file changed to %q", got)
		}
	})
}

func TestIsUploadArtifact(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/tmp/1agents-jay_style_v1-3982749823.mp3", true},
		{"/tmp/1agents-foo-abc123.txt", true},
		{"/tmp/1agents-upload.mp3", true},
		{"/tmp/jay_style_v1.mp3", false}, // renamed: lost "1agents-" prefix
		{"/tmp/1agents-../etc/passwd", false},
		{"/etc/passwd", false},
		{"/var/tmp/1agents-x.dat", false}, // only /tmp, not /var/tmp
		{"/tmp/", false},
	}
	for _, c := range cases {
		if got := isUploadArtifact(c.path); got != c.want {
			t.Errorf("isUploadArtifact(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestSafeAbs_AllowsUploadArtifact(t *testing.T) {
	// Point meta.db at an isolated empty home so checkWorkspaces returns
	// false (no registered workspaces), forcing safeAbs to fall through to
	// the upload-artifact branch.
	isolatedHome := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", isolatedHome)
	t.Setenv("HOME", isolatedHome)
	t.Setenv("USERPROFILE", isolatedHome)

	// Create a fake upload artifact matching CreateTemp("/tmp", "1agents-...-*")
	uploaded, err := os.CreateTemp("/tmp", "1agents-safetest-*")
	if err != nil {
		t.Fatalf("create upload artifact: %v", err)
	}
	defer os.Remove(uploaded.Name())
	uploaded.WriteString("payload")
	uploaded.Close()

	h := &Handler{root: filepath.Join(isolatedHome, "nonexistent-root")}

	t.Run("absolute upload artifact path is allowed", func(t *testing.T) {
		abs, ok := h.safeAbs(uploaded.Name())
		if !ok {
			t.Fatalf("expected upload artifact %q to be allowed, got rejected", uploaded.Name())
		}
		if !strings.HasPrefix(abs, "/tmp/1agents-safetest-") {
			t.Errorf("expected path under /tmp, got %q", abs)
		}
	})

	t.Run("renamed file without 1agents- prefix is still rejected", func(t *testing.T) {
		// Sanity check: the whitelist is strict, so a renamed artifact
		// (e.g. /tmp/jay_style_v1.mp3) still falls through to 403.
		_, ok := h.safeAbs("/tmp/jay_style_v1.mp3")
		if ok {
			t.Error("expected /tmp/jay_style_v1.mp3 to remain forbidden (no 1agents- prefix)")
		}
	})

	t.Run("non-/tmp absolute path remains forbidden", func(t *testing.T) {
		_, ok := h.safeAbs("/etc/passwd")
		if ok {
			t.Error("expected /etc/passwd to remain forbidden")
		}
	})
}
