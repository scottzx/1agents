package sources

// bearer_credentials.go stores static Bearer tokens for manifest REST sources.
// Unlike iCloud (macOS Keychain — inherently an Apple-local concern), a generic
// REST token has no reason to be OS-bound, and the product deploys on Linux, so
// tokens live in a per-account file under ~/.1agents/sources/<vendor>_tokens
// (mode 0600), keyed by bronze account_id — the same shape as the MS OAuth store,
// naturally multi-account and cross-platform.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type storedBearer struct {
	Token     string `json:"token"`
	UpdatedAt string `json:"updatedAt"`
}

func bearerDir(vendor string) string { return filepath.Join(sourcesHome(), vendor+"_tokens") }

func bearerPath(vendor, accountID string) string {
	return filepath.Join(bearerDir(vendor), accountBearerFile(accountID))
}

func accountBearerFile(accountID string) string {
	if accountID == "" {
		accountID = "default"
	}
	return accountID + ".json"
}

// SaveBearerToken persists a token for (vendor, accountID) at 0600 (dir 0700).
func SaveBearerToken(vendor, accountID, token string) error {
	dir := bearerDir(vendor)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(storedBearer{Token: token, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bearerPath(vendor, accountID), b, 0o600)
}

// LoadBearerToken returns the stored token; ok=false when never configured.
func LoadBearerToken(vendor, accountID string) (string, bool, error) {
	b, err := os.ReadFile(bearerPath(vendor, accountID))
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var t storedBearer
	if err := json.Unmarshal(b, &t); err != nil {
		return "", false, err
	}
	if t.Token == "" {
		return "", false, nil
	}
	return t.Token, true, nil
}

// DeleteBearerToken removes a stored token (best-effort).
func DeleteBearerToken(vendor, accountID string) {
	_ = os.Remove(bearerPath(vendor, accountID))
}

// BearerConfigured reports whether a token is stored for (vendor, accountID).
func BearerConfigured(vendor, accountID string) bool {
	_, ok, _ := LoadBearerToken(vendor, accountID)
	return ok
}
