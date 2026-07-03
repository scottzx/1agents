package workspace

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// presetAvatarsFS carries the built-in assistant avatar set (resized to 256px).
// Lives under presets/avatars/ (sibling to presets/souls/) and is served under
// /avatars/presets/<name>.png so the frontend preset picker can reference them
// as plain avatar URLs.
//
//go:embed presets/avatars/*.png
var presetAvatarsFS embed.FS

// avatarsDir is where uploaded assistant avatars live on disk. Served by
// GET /avatars/... (see ServeAvatars).
func avatarsDir() string {
	return filepath.Join(get1AgentsHome(), ".1agents", "avatars")
}

// randomAvatarName returns an unpredictable file basename (no extension).
func randomAvatarName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// UploadAvatar handles POST /api/workspace/upload-avatar (multipart/form-data).
// Field name: "file". Copies into ~/.1agents/avatars/ and returns {url}.
func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// 10 MiB cap — assistant avatars are small; anything larger is a mistake.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		// ok
	default:
		http.Error(w, "unsupported image type", http.StatusUnsupportedMediaType)
		return
	}
	if err := os.MkdirAll(avatarsDir(), 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := randomAvatarName() + ext
	dest := filepath.Join(avatarsDir(), name)
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		os.Remove(dest)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := out.Close(); err != nil {
		os.Remove(dest)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"url": "/avatars/" + name})
}

// ServeAvatars returns an http.Handler for /avatars/: the embedded preset set
// under /avatars/presets/, everything else from ~/.1agents/avatars (uploads).
// Registered as a top-level mux route so the URL stays outside /api.
func ServeAvatars() http.Handler {
	// Ensure the upload directory exists so FileServer doesn't 404 on
	// cold-start empty installs.
	_ = os.MkdirAll(avatarsDir(), 0o755)
	disk := http.StripPrefix("/avatars/", http.FileServer(http.Dir(avatarsDir())))
	presets, _ := fs.Sub(presetAvatarsFS, "presets/avatars")
	presetSrv := http.StripPrefix("/avatars/presets/", http.FileServer(http.FS(presets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/avatars/presets/") {
			presetSrv.ServeHTTP(w, r)
			return
		}
		disk.ServeHTTP(w, r)
	})
}
