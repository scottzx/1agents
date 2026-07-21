package system

// End-to-end tests for the OTA three-layer update feature (issue #52).
//
// These tests exercise the *update pipeline* against live in-process HTTP
// servers (httptest) rather than mocking individual functions, so they
// cover the same code paths a real update would take:
//
//   - Layer 1 (frontend web update): the /api/system/version handler and
//     getLatestVersion read a served manifest and decide hasUpdate.
//   - Layer 2 (Go backend self-update): manifest fetch with mirror→GitHub
//     fallback, platform-binary resolution, tarball download, SHA256
//     verification, and extraction of the correct binary from a real
//     .tar.gz that may contain sibling binaries.
//
// We deliberately stop short of calling selfupdate.Apply / runUpdate's
// restart stage: Apply replaces os.Executable(), which in `go test` is the
// test binary itself. The download→verify→extract pipeline above is the
// part unique to OTA; the final atomic-swap is delegated to the vetted
// minio/selfupdate library and is not re-tested here.
//
// Layer 3 (Tauri desktop) is Rust/JS (tauri-plugin-updater) and the
// manifest-build scripts have their own coverage in ota-verify.yml; both
// are out of scope for Go tests.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeTarGz builds an in-memory .tar.gz containing the given files
// (name → content) and returns the gzipped bytes plus their SHA256.
func makeTarGz(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

// ── Layer 2: extractBinaryFromTarGz ──────────────────────────────────────────

func TestExtractBinaryFromTarGz_PicksCorrectEntry(t *testing.T) {
	// The tarball ships three sibling binaries; only "1agents" must be
	// extracted (V1 swaps a single binary — see extractBinaryFromTarGz doc).
	archive, _ := makeTarGz(t, map[string]string{
		"1agents-v1/ttyd":       "TTYD-BINARY",
		"1agents-v1/cc-connect": "CCCONNECT-BINARY",
		"1agents-v1/1agents":    "REAL-AGENT-BINARY",
	})

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "release.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	out, err := extractBinaryFromTarGz(archivePath, dstDir)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(got) != "REAL-AGENT-BINARY" {
		t.Errorf("extracted wrong entry: %q", got)
	}
	// Must land inside the requested destination dir.
	if filepath.Dir(out) != dstDir {
		t.Errorf("extracted to %q, want inside %q", out, dstDir)
	}
	// Exec bit preserved on unix.
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(out)
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("extracted binary not executable: %v", info.Mode())
		}
	}
}

func TestExtractBinaryFromTarGz_MissingEntry(t *testing.T) {
	archive, _ := makeTarGz(t, map[string]string{
		"some-dir/ttyd": "only-a-sibling",
	})
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "release.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := extractBinaryFromTarGz(archivePath, t.TempDir()); err == nil {
		t.Fatal("expected error when archive lacks the 1agents entry")
	}
}

func TestExtractBinaryFromTarGz_NotAGzip(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "garbage.tar.gz")
	if err := os.WriteFile(bad, []byte("this is not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := extractBinaryFromTarGz(bad, t.TempDir()); err == nil {
		t.Fatal("expected gunzip error on non-gzip input")
	}
}

// ── Layer 2: download + SHA256 verify pipeline ───────────────────────────────

// downloadAndVerify mirrors the download/hash stage of runUpdate against a
// served URL. We test this stage in isolation because runUpdate's tail end
// (selfupdate.Apply + restart) would replace the test binary.
func downloadAndVerify(url, expectedSHA string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "1agents-ota-test-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), resp.Body); err != nil {
		return "", err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA != "" && got != expectedSHA {
		return "", fmt.Errorf("SHA256 mismatch: got %s, want %s", got, expectedSHA)
	}
	return tmp.Name(), nil
}

func TestDownloadAndVerify_RoundTrip(t *testing.T) {
	archive, wantSHA := makeTarGz(t, map[string]string{"r/1agents": "AGENT"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()

	path, err := downloadAndVerify(srv.URL, wantSHA)
	if err != nil {
		t.Fatalf("download+verify: %v", err)
	}
	defer os.Remove(path)

	// And the downloaded tarball must be extractable end-to-end.
	out, err := extractBinaryFromTarGz(path, t.TempDir())
	if err != nil {
		t.Fatalf("extract after download: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "AGENT" {
		t.Errorf("extracted %q after download round-trip", got)
	}
}

func TestDownloadAndVerify_SHAMismatchRejected(t *testing.T) {
	archive, _ := makeTarGz(t, map[string]string{"r/1agents": "AGENT"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()

	const wrongSHA = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := downloadAndVerify(srv.URL, wrongSHA); err == nil {
		t.Fatal("expected SHA256 mismatch to be rejected")
	}
}

// ── Layer 2: manifest fetch with mirror → GitHub fallback ────────────────────

func TestFetchUpstream_BrokenSourceThenHealthy(t *testing.T) {
	// fetchUpstream tries each configured source. We can't point the
	// GitHub fallback at a local server (it's a hard-coded github.com
	// URL), so we exercise the failure→recovery behaviour through the
	// mirror slot: a broken mirror with no other source must fail, and a
	// healthy mirror must return its manifest body.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer broken.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"channel":"stable","components":{"backend":{"version":"HEALTHY"}}}`))
	}))
	defer healthy.Close()

	oldMirror, oldRepo := MirrorBaseURL, Repo
	t.Cleanup(func() { MirrorBaseURL, Repo = oldMirror, oldRepo })
	Repo = "" // disable real GitHub fallback so the test is deterministic

	MirrorBaseURL = broken.URL
	if _, err := fetchUpstream(); err == nil {
		t.Fatal("expected failure when the only source is broken (500)")
	}

	MirrorBaseURL = healthy.URL
	body, err := fetchUpstream()
	if err != nil {
		t.Fatalf("fetchUpstream with healthy source: %v", err)
	}
	if !strings.Contains(string(body), "HEALTHY") {
		t.Errorf("expected healthy manifest body, got %q", body)
	}
}

func TestFetchUpstream_PrefersNewerAcrossSources(t *testing.T) {
	// Regression: stale COS (v20260718-8) must not mask a fresher GitHub
	// release (v20260720-2) when both have platforms.
	stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"channel":"stable","components":{"backend":{"version":"v20260718-8","platforms":{"linux-amd64":{"url":"http://stale"}}}}}`))
	}))
	defer stale.Close()
	fresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"channel":"stable","components":{"backend":{"version":"v20260720-2","platforms":{"linux-amd64":{"url":"http://fresh"}}}}}`))
	}))
	defer fresh.Close()

	oldMirror, oldRepo := MirrorBaseURL, Repo
	t.Cleanup(func() { MirrorBaseURL, Repo = oldMirror, oldRepo })
	MirrorBaseURL = stale.URL
	Repo = fresh.URL + "/manifest.json"

	body, err := fetchUpstream()
	if err != nil {
		t.Fatalf("fetchUpstream: %v", err)
	}
	if got := backendVersionOf(body); got != "v20260720-2" {
		t.Errorf("backend version = %q, want v20260720-2 (fresher source)", got)
	}
	if !strings.Contains(string(body), "http://fresh") {
		t.Errorf("expected fresher manifest body, got %s", body)
	}
}

func TestFetchUpstream_SkipsEmptyPlatformsNewer(t *testing.T) {
	// Broken GH manifest after npm-split: version bumps but platforms={}.
	// Keep the older usable COS mirror so OTA still has a download URL.
	usable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"channel":"stable","components":{"backend":{"version":"v20260718-8","platforms":{"linux-amd64":{"url":"http://usable"}}}}}`))
	}))
	defer usable.Close()
	emptyNewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"channel":"stable","components":{"backend":{"version":"v20260720-2","platforms":{}}}}`))
	}))
	defer emptyNewer.Close()

	oldMirror, oldRepo := MirrorBaseURL, Repo
	t.Cleanup(func() { MirrorBaseURL, Repo = oldMirror, oldRepo })
	MirrorBaseURL = usable.URL
	Repo = emptyNewer.URL + "/manifest.json"

	body, err := fetchUpstream()
	if err != nil {
		t.Fatalf("fetchUpstream: %v", err)
	}
	if got := backendVersionOf(body); got != "v20260718-8" {
		t.Errorf("backend version = %q, want v20260718-8 (usable over empty newer)", got)
	}
	if !strings.Contains(string(body), "http://usable") {
		t.Errorf("expected usable manifest body, got %s", body)
	}
}

func TestFetchManifestFrom_RejectsHTMLErrorPage(t *testing.T) {
	// CDN edges sometimes return HTTP 200 with an HTML 404 body. The
	// validator must reject anything that isn't a manifest-shaped JSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>404 Not Found</body></html>"))
	}))
	defer srv.Close()
	if _, err := fetchManifestFrom(srv.URL + "/manifest.json"); err == nil {
		t.Fatal("expected rejection of non-JSON 200 body")
	}

	// A 200 JSON that lacks "components" must also be rejected.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"channel":"stable"}`))
	}))
	defer srv2.Close()
	if _, err := fetchManifestFrom(srv2.URL + "/manifest.json"); err == nil {
		t.Fatal("expected rejection of JSON missing 'components'")
	}
}

// ── Layer 1 (web update) + Layer 2: version handler hasUpdate decision ────────

func servedManifest(backendVersion string) string {
	m := RootManifest{Channel: "stable"}
	m.Components.Frontend = FrontendComponent{Version: backendVersion}
	m.Components.Backend = BackendComponent{
		Version: backendVersion,
		Platforms: map[string]PlatformBinary{
			platformKey(): {URL: "https://example/bin.tar.gz", SHA256: "deadbeef"},
		},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func withMirror(t *testing.T, manifestJSON string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(manifestJSON))
	}))
	t.Cleanup(srv.Close)

	oldMirror, oldRepo := MirrorBaseURL, Repo
	t.Cleanup(func() { MirrorBaseURL, Repo = oldMirror, oldRepo })
	MirrorBaseURL = srv.URL
	Repo = "" // disable real GitHub
}

func TestVersionEndpoint_ReportsUpdateAvailable(t *testing.T) {
	oldLV := LocalVersion
	t.Cleanup(func() { LocalVersion = oldLV })
	LocalVersion = "20260101-1" // older than served

	withMirror(t, servedManifest("20260615-9"))

	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/system/version", nil)
	rec := httptest.NewRecorder()
	h.Version(rec, req)

	var info VersionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Latest != "20260615-9" {
		t.Errorf("latest = %q, want 20260615-9", info.Latest)
	}
	if !info.HasUpdate {
		t.Errorf("has_update = false, want true (local %s < latest %s)", info.Current, info.Latest)
	}
}

func TestVersionEndpoint_NoUpdateWhenCurrent(t *testing.T) {
	oldLV := LocalVersion
	t.Cleanup(func() { LocalVersion = oldLV })
	LocalVersion = "20260615-9"

	withMirror(t, servedManifest("20260615-9"))

	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/system/version", nil)
	rec := httptest.NewRecorder()
	h.Version(rec, req)

	var info VersionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.HasUpdate {
		t.Errorf("has_update = true, want false when local == latest")
	}
}

// ── Layer 2: platform binary resolution against a served manifest ────────────

func TestPlatformBinaryURL_ResolvesCurrentPlatform(t *testing.T) {
	// Build a manifest that *includes* the current platform so resolution
	// succeeds end-to-end (the existing TestPlatformBinaryURL only covers
	// the absent-platform error path).
	body := []byte(servedManifest("20260615-1"))
	url, sha, err := platformBinaryURL(body)
	if err != nil {
		t.Fatalf("resolve current platform %s: %v", platformKey(), err)
	}
	if url == "" || sha == "" {
		t.Errorf("empty url/sha: url=%q sha=%q", url, sha)
	}
}
