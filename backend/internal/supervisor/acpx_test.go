package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcpxEnvironmentPrependsUserAgentBinDirectories(t *testing.T) {
	home := filepath.Join(string(os.PathSeparator), "Users", "test")
	base := []string{"PATH=/usr/bin:/bin", "KEEP=unchanged"}

	got := acpxEnvironment(base, home)
	values := environmentValues(got)
	wantPath := strings.Join([]string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".grok", "bin"),
		"/usr/bin:/bin",
	}, string(os.PathListSeparator))

	if values["PATH"] != wantPath {
		t.Fatalf("PATH = %q, want %q", values["PATH"], wantPath)
	}
	if values["KEEP"] != "unchanged" {
		t.Fatalf("KEEP = %q, want unchanged", values["KEEP"])
	}
}

func TestAcpxEnvironmentAvoidsDuplicateUserAgentBinDirectories(t *testing.T) {
	home := filepath.Join(string(os.PathSeparator), "Users", "test")
	localBin := filepath.Join(home, ".local", "bin")
	grokBin := filepath.Join(home, ".grok", "bin")
	base := []string{"PATH=" + strings.Join([]string{localBin, "/usr/bin", grokBin}, string(os.PathListSeparator))}

	got := environmentValues(acpxEnvironment(base, home))["PATH"]
	parts := strings.Split(got, string(os.PathListSeparator))

	if countPath(parts, localBin) != 1 {
		t.Fatalf("PATH contains %q %d times, want once: %q", localBin, countPath(parts, localBin), got)
	}
	if countPath(parts, grokBin) != 1 {
		t.Fatalf("PATH contains %q %d times, want once: %q", grokBin, countPath(parts, grokBin), got)
	}
}

func environmentValues(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	return values
}

func countPath(paths []string, target string) int {
	count := 0
	for _, path := range paths {
		if path == target {
			count++
		}
	}
	return count
}

func TestResolveAcpBridgeUsesServiceSource(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "services", "acp-service", "bin", "acp-service.ts")
	if err := os.MkdirAll(filepath.Dir(entry), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("// fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "1agents_app", "backend")
	if err := os.MkdirAll(app, 0700); err != nil {
		t.Fatal(err)
	}
	found, err := resolveAcpBridge(app)
	if err != nil {
		t.Fatal(err)
	}
	if found.script != entry || !found.useTsx || !found.service {
		t.Fatalf("wrong service launch: %+v", found)
	}
}
