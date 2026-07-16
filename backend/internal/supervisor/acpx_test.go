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
