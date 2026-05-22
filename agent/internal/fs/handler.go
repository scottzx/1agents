package fs

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Handler handles all file-system CRUD operations within a sandboxed root
// directory. Requests that attempt to escape the root via path traversal
// (e.g. "../../etc/passwd") are rejected with 403 Forbidden.
type Handler struct {
	root string // absolute, cleaned root path
}

// NewHandler creates a Handler whose root is the given directory.
// The path is resolved to an absolute path so that all security checks
// are reliable regardless of the working directory at call time.
func NewHandler(root string) *Handler {
	abs, err := filepath.Abs(root)
	if err != nil {
		log.Fatalf("[fs] cannot resolve root path %q: %v", root, err)
	}
	return &Handler{root: abs}
}

// SetRoot changes the sandbox root directory at runtime.
// Returns an error if the path cannot be resolved.
func (h *Handler) SetRoot(newRoot string) error {
	abs, err := filepath.Abs(newRoot)
	if err != nil {
		return err
	}
	h.root = abs
	log.Printf("[fs] root changed to %q", abs)
	return nil
}

// Root returns the current sandbox root directory.
func (h *Handler) Root() string {
	return h.root
}

// --- Entry types ---

// FileEntry is the JSON representation of a single file or directory.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // relative to root
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"modTime"` // Unix timestamp (seconds)
}

// --- Public HTTP handlers ---

// List handles GET /api/fs/list?path=<relative-path>
// Returns the immediate children of the given directory as a JSON array.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := r.URL.Query().Get("path")
	abs, ok := h.safeAbs(rel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "directory not found", http.StatusNotFound)
			return
		}
		if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "not a directory") {
			http.Error(w, "path is not a directory", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		// Skip OS/editor noise files only – dot-directories like .github, .claude, .env etc. are kept.
		name := e.Name()
		if name == ".DS_Store" || name == ".Spotlight-V100" || name == ".Trashes" || name == ".fseventsd" {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		entryRel, _ := filepath.Rel(h.root, filepath.Join(abs, e.Name()))
		result = append(result, FileEntry{
			Name:    e.Name(),
			Path:    filepath.ToSlash(entryRel),
			IsDir:   e.IsDir(),
			Size:    sizeOf(info),
			ModTime: info.ModTime().Unix(),
		})
	}

	writeJSON(w, result)
}

// Read handles GET /api/fs/read?path=<relative-path>
// Returns the raw file content as plain text.
// Files larger than 10 MB are rejected to protect server memory.
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := r.URL.Query().Get("path")
	abs, ok := h.safeAbs(rel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "path is a directory", http.StatusBadRequest)
		return
	}
	const maxBytes = 10 << 20 // 10 MB
	if info.Size() > maxBytes {
		http.Error(w, "file too large (>10 MB)", http.StatusRequestEntityTooLarge)
		return
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// Image handles GET /api/fs/image?path=<relative-path>
// Returns the file as a base64-encoded data URL for image preview.
// Supported formats: gif, png, jpg, jpeg, webp, bmp, svg
func (h *Handler) Image(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := r.URL.Query().Get("path")
	abs, ok := h.safeAbs(rel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "path is a directory", http.StatusBadRequest)
		return
	}

	const maxBytes = 10 << 20 // 10 MB
	if info.Size() > maxBytes {
		http.Error(w, "file too large (>10 MB)", http.StatusRequestEntityTooLarge)
		return
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	mimeType := getImageMimeType(abs)
	encoded := base64.StdEncoding.EncodeToString(data)
	dataURL := "data:" + mimeType + ";base64," + encoded

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(dataURL))
}

// getImageMimeType returns the MIME type for common image formats
func getImageMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".gif":
		return "image/gif"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// Write handles POST /api/fs/write?path=<relative-path>
// The request body is written verbatim to the file, creating it if necessary.
func (h *Handler) Write(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := r.URL.Query().Get("path")
	abs, ok := h.safeAbs(rel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	const maxBytes = 10 << 20 // 10 MB
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if int64(len(body)) > maxBytes {
		http.Error(w, "request body too large (>10 MB)", http.StatusRequestEntityTooLarge)
		return
	}

	if err := os.WriteFile(abs, body, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// Mkdir handles POST /api/fs/mkdir?path=<relative-path>
// Creates the directory (and all parents) at the given path.
func (h *Handler) Mkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := r.URL.Query().Get("path")
	abs, ok := h.safeAbs(rel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// Delete handles DELETE /api/fs/delete?path=<relative-path>
// Removes a file or an empty directory. Use ?recursive=true to remove
// a non-empty directory tree.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := r.URL.Query().Get("path")
	abs, ok := h.safeAbs(rel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Safety: refuse to delete the root itself.
	if abs == h.root {
		http.Error(w, "cannot delete the workspace root", http.StatusForbidden)
		return
	}

	recursive := r.URL.Query().Get("recursive") == "true"

	var err error
	if recursive {
		err = os.RemoveAll(abs)
	} else {
		err = os.Remove(abs)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// --- Helpers ---

// safeAbs resolves a relative path against the root and verifies the result
// is still within the root (path traversal guard).
// Returns the absolute path and true on success, or "" and false if the path
// attempts to escape the sandbox.
func (h *Handler) safeAbs(rel string) (string, bool) {
	// filepath.Join cleans ".." components.
	joined := filepath.Join(h.root, filepath.FromSlash(rel))
	cleaned := filepath.Clean(joined)

	// The cleaned path must start with root + separator (or equal root).
	if cleaned != h.root && !strings.HasPrefix(cleaned, h.root+string(os.PathSeparator)) {
		log.Printf("[fs] path traversal blocked: %q resolved to %q", rel, cleaned)
		return "", false
	}
	return cleaned, true
}

// sizeOf returns the size of a file, or 0 for directories.
func sizeOf(info fs.FileInfo) int64 {
	if info.IsDir() {
		return 0
	}
	return info.Size()
}

// writeJSON serialises v to JSON and writes it to w with the correct
// Content-Type header.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[fs] json encode error: %v", err)
	}
}
