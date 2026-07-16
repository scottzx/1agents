package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAgentCatalogIncludesGrokBuild(t *testing.T) {
	for _, descriptor := range AgentCatalog {
		if descriptor.Type != AgentTypeGrokBuild {
			continue
		}
		if descriptor.Binary != "grok" {
			t.Fatalf("Grok binary = %q, want grok", descriptor.Binary)
		}
		if !descriptor.Integrated || !descriptor.AcpCapable || descriptor.CcTransport != TransportACP {
			t.Fatalf("Grok descriptor is not ACP chat-ready: %+v", descriptor)
		}
		if descriptor.AdapterPackage != "" {
			t.Fatalf("Grok must require its installed CLI, got adapter %q", descriptor.AdapterPackage)
		}
		return
	}
	t.Fatal("Grok Build missing from AgentCatalog")
}

func TestCatalogScanFindsAgentsInUserBinDirsOutsideProcessPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("user-local bin fallback is for Unix-like hosts")
	}

	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	grokDir := filepath.Join(home, ".grok", "bin")
	if err := os.MkdirAll(grokDir, 0o755); err != nil {
		t.Fatal(err)
	}
	grokPath := filepath.Join(grokDir, "grok")
	if err := os.WriteFile(grokPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	statuses := (&CatalogStore{}).Scan()
	wantPaths := map[AgentType]string{
		AgentTypeClaudecode: claudePath,
		AgentTypeGrokBuild:  grokPath,
	}
	for _, status := range statuses {
		wantPath, ok := wantPaths[status.Type]
		if !ok {
			continue
		}
		if !status.Installed || status.Path != wantPath {
			t.Errorf("%s status = %+v, want installed path %q", status.Type, status, wantPath)
		}
		delete(wantPaths, status.Type)
	}
	if len(wantPaths) != 0 {
		t.Fatalf("agents missing from scanned catalog: %v", wantPaths)
	}
}
