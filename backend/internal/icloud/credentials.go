package icloud

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// keychainService is the macOS Keychain generic-password service under which the
// app-specific password is stored.
const keychainService = "1agents-icloud"

// home resolves ~/.1agents (honoring ONEAGENTS_HOME), the local config root.
func home() string {
	base := os.Getenv("ONEAGENTS_HOME")
	if base == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			base = "."
		} else {
			base = h
		}
	}
	return filepath.Join(base, ".1agents")
}

func accountPath() string { return filepath.Join(home(), "icloud.json") }

type account struct {
	AppleID string `json:"appleId"`
}

// SaveCredentials stores the Apple ID locally and the app-specific password in
// the macOS Keychain (the password never touches our config file). macOS only.
func SaveCredentials(appleID, password string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("icloud: credential storage currently supports macOS only")
	}
	appleID = strings.TrimSpace(appleID)
	if appleID == "" || strings.TrimSpace(password) == "" {
		return fmt.Errorf("icloud: appleID and password are required")
	}
	if err := os.MkdirAll(home(), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(account{AppleID: appleID})
	if err := os.WriteFile(accountPath(), data, 0o600); err != nil {
		return err
	}
	// -U updates in place; -T /usr/bin/security puts the security tool on the
	// item ACL so our later reads via `security` don't trigger a Keychain prompt.
	cmd := exec.Command("security", "add-generic-password",
		"-U", "-s", keychainService, "-a", appleID, "-w", password,
		"-T", "/usr/bin/security")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("icloud: keychain store failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SaveKeychainPassword stores an app-specific password in the macOS Keychain
// keyed by Apple ID. Because the Keychain item's account is the Apple ID, this
// is naturally multi-account — the data-source account model (source_accounts)
// stores the Apple ID + region and calls this to persist the secret, without
// touching the legacy single-account icloud.json path. macOS only.
func SaveKeychainPassword(appleID, password string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("icloud: credential storage currently supports macOS only")
	}
	appleID = strings.TrimSpace(appleID)
	if appleID == "" || strings.TrimSpace(password) == "" {
		return fmt.Errorf("icloud: appleID and password are required")
	}
	cmd := exec.Command("security", "add-generic-password",
		"-U", "-s", keychainService, "-a", appleID, "-w", password,
		"-T", "/usr/bin/security")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("icloud: keychain store failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// LoadKeychainPassword returns the app-specific password for an Apple ID. ok is
// false when no Keychain item exists.
func LoadKeychainPassword(appleID string) (password string, ok bool, err error) {
	if runtime.GOOS != "darwin" {
		return "", false, nil
	}
	out, cerr := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-a", appleID, "-w").Output()
	if cerr != nil {
		return "", false, nil
	}
	return strings.TrimRight(string(out), "\n"), true, nil
}

// DeleteKeychainPassword removes the Keychain item for an Apple ID (best-effort).
func DeleteKeychainPassword(appleID string) error {
	if runtime.GOOS != "darwin" || strings.TrimSpace(appleID) == "" {
		return nil
	}
	return exec.Command("security", "delete-generic-password",
		"-s", keychainService, "-a", appleID).Run()
}

// LoadCredentials returns the stored Apple ID + app-specific password. ok is
// false when no credential is configured.
func LoadCredentials() (appleID, password string, ok bool, err error) {
	if runtime.GOOS != "darwin" {
		return "", "", false, nil
	}
	data, rerr := os.ReadFile(accountPath())
	if os.IsNotExist(rerr) {
		return "", "", false, nil
	}
	if rerr != nil {
		return "", "", false, rerr
	}
	var a account
	if jerr := json.Unmarshal(data, &a); jerr != nil || a.AppleID == "" {
		return "", "", false, nil
	}
	out, cerr := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-a", a.AppleID, "-w").Output()
	if cerr != nil {
		// Account file present but no keychain item — treat as not configured.
		return "", "", false, nil
	}
	return a.AppleID, strings.TrimRight(string(out), "\n"), true, nil
}

// Status reports the configured Apple ID (no password) for the settings UI.
func Status() (appleID string, configured bool) {
	id, _, ok, _ := LoadCredentials()
	return id, ok
}

// ClearCredentials removes the stored Apple ID and Keychain password.
func ClearCredentials() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	var a account
	if data, err := os.ReadFile(accountPath()); err == nil {
		_ = json.Unmarshal(data, &a)
	}
	_ = os.Remove(accountPath())
	if a.AppleID != "" {
		_ = exec.Command("security", "delete-generic-password",
			"-s", keychainService, "-a", a.AppleID).Run()
	}
	return nil
}
