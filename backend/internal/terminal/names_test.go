package terminal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// withTempHome redirects os.UserHomeDir to a fresh temp dir for the duration
// of the test, so the names file lands in a sandboxed location.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })
	return dir
}

func TestLoadSessionNames_MissingFile(t *testing.T) {
	withTempHome(t)

	got, err := loadSessionNames()
	if err != nil {
		t.Fatalf("expected no error on missing file, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestSetSessionName_NewEntry(t *testing.T) {
	withTempHome(t)
	h := &Handler{}

	if err := h.SetSessionName("wsId_1", "My Cool Project"); err != nil {
		t.Fatalf("SetSessionName error: %v", err)
	}

	got, err := loadSessionNames()
	if err != nil {
		t.Fatalf("loadSessionNames error: %v", err)
	}
	if got["wsId_1"] != "My Cool Project" {
		t.Fatalf("expected wsId_1=My Cool Project, got %q", got["wsId_1"])
	}
}

func TestSetSessionName_Overwrite(t *testing.T) {
	withTempHome(t)
	h := &Handler{}

	if err := h.SetSessionName("wsId_1", "First"); err != nil {
		t.Fatalf("SetSessionName first error: %v", err)
	}
	if err := h.SetSessionName("wsId_1", "Second"); err != nil {
		t.Fatalf("SetSessionName second error: %v", err)
	}

	got, err := loadSessionNames()
	if err != nil {
		t.Fatalf("loadSessionNames error: %v", err)
	}
	if got["wsId_1"] != "Second" {
		t.Fatalf("expected wsId_1=Second (overwrite), got %q", got["wsId_1"])
	}
}

func TestSetSessionName_EmptyDeletes(t *testing.T) {
	withTempHome(t)
	h := &Handler{}

	if err := h.SetSessionName("wsId_1", "Foo"); err != nil {
		t.Fatalf("SetSessionName error: %v", err)
	}
	if err := h.SetSessionName("wsId_1", ""); err != nil {
		t.Fatalf("SetSessionName empty error: %v", err)
	}

	got, err := loadSessionNames()
	if err != nil {
		t.Fatalf("loadSessionNames error: %v", err)
	}
	if _, present := got["wsId_1"]; present {
		t.Fatalf("expected wsId_1 to be deleted, got %v", got)
	}
}

func TestSetSessionName_DeleteSessionName(t *testing.T) {
	withTempHome(t)
	h := &Handler{}

	_ = h.SetSessionName("wsId_1", "Foo")
	_ = h.SetSessionName("wsId_2", "Bar")
	if err := h.DeleteSessionName("wsId_1"); err != nil {
		t.Fatalf("DeleteSessionName error: %v", err)
	}

	got, err := loadSessionNames()
	if err != nil {
		t.Fatalf("loadSessionNames error: %v", err)
	}
	if _, present := got["wsId_1"]; present {
		t.Fatalf("expected wsId_1 to be deleted, got %v", got)
	}
	if got["wsId_2"] != "Bar" {
		t.Fatalf("expected wsId_2 to remain, got %v", got)
	}
}

func TestLoadSessionNames_CorruptionRecovery(t *testing.T) {
	dir := withTempHome(t)
	path := filepath.Join(dir, ".1agents", "session_names.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	got, err := loadSessionNames()
	if err != nil {
		t.Fatalf("expected no error on corrupt file, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map on corruption, got %v", got)
	}

	// File should NOT be deleted by the loader — never destroy user data on
	// a parse failure.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to remain after corruption recovery, got stat error: %v", err)
	}
}

func TestApplySessionNames_Overlay(t *testing.T) {
	withTempHome(t)
	h := &Handler{}

	_ = h.SetSessionName("wsId_1", "Renamed One")
	_ = h.SetSessionName("wsId_3", "Renamed Three")

	windows := []TmuxWindow{
		{Index: 0, Name: "wsId_1"},
		{Index: 1, Name: "wsId_2"},
		{Index: 2, Name: "wsId_3"},
	}

	out := h.ApplySessionNames(windows)
	if out[0].CustomName != "Renamed One" {
		t.Fatalf("expected wsId_1.CustomName=Renamed One, got %q", out[0].CustomName)
	}
	if out[1].CustomName != "" {
		t.Fatalf("expected wsId_2.CustomName empty, got %q", out[1].CustomName)
	}
	if out[2].CustomName != "Renamed Three" {
		t.Fatalf("expected wsId_3.CustomName=Renamed Three, got %q", out[2].CustomName)
	}
}

func TestSessionNamesFile_PersistedShape(t *testing.T) {
	dir := withTempHome(t)
	h := &Handler{}

	if err := h.SetSessionName("wsId_1", "Hello"); err != nil {
		t.Fatalf("SetSessionName error: %v", err)
	}

	path := filepath.Join(dir, ".1agents", "session_names.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	var cfg sessionNamesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal persisted file: %v", err)
	}
	if cfg.Names["wsId_1"] != "Hello" {
		t.Fatalf("expected persisted wsId_1=Hello, got %q", cfg.Names["wsId_1"])
	}
}
