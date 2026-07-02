package ingest

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// accounts_http.go serves the 数据源账号 (source account) registry — the backbone
// of the 源为中心 model where 厂家 + 账号 = 一个源. Region (国际/大陆) is an explicit
// account property fixed at creation (solving Apple's implicit-region problem),
// and single-account vendors (飞书) are enforced here.
//
//	GET  /api/sources/vendors       → vendor capability table (add-flow drives off this)
//	GET  /api/sources/accounts      → every registered account
//	POST /api/sources/accounts      → create one account (+ vendor-specific secret)
//	DELETE /api/sources/accounts/{id} → remove an account

// HandleVendors serves GET /api/sources/vendors.
func (h *Handler) HandleVendors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, sources.Vendors)
}

// createAccountReq is the POST body. Secret fields are vendor-specific: iCloud
// uses appleId+password (stored in the Keychain, never persisted here); OAuth
// vendors (microsoft/google) carry no secret yet — the account is a placeholder
// until the OAuth flow lands.
type createAccountReq struct {
	Vendor   string `json:"vendor"`
	Region   string `json:"region"`
	Label    string `json:"label"`
	AppleID  string `json:"appleId"`
	Password string `json:"password"`
}

// HandleAccounts serves GET/POST /api/sources/accounts.
func (h *Handler) HandleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.accounts.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		h.createAccount(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var body createAccountReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	body.Vendor = strings.TrimSpace(body.Vendor)
	body.Region = strings.TrimSpace(body.Region)
	spec := sources.VendorFor(body.Vendor)
	if spec == nil {
		http.Error(w, "unknown vendor", http.StatusBadRequest)
		return
	}
	if body.Region == "" && len(spec.Regions) > 0 {
		body.Region = spec.Regions[0]
	}
	if !sources.RegionAllowed(body.Vendor, body.Region) {
		http.Error(w, "region not supported for vendor", http.StatusBadRequest)
		return
	}

	label := strings.TrimSpace(body.Label)
	// iCloud: the Apple ID is the label and the secret goes to the Keychain
	// (keyed by Apple ID → naturally multi-account). Required before we register
	// the account so a listed account always has a usable credential.
	if body.Vendor == meta.VendorICloud {
		appleID := strings.TrimSpace(body.AppleID)
		if appleID == "" || strings.TrimSpace(body.Password) == "" {
			http.Error(w, "appleId and password are required", http.StatusBadRequest)
			return
		}
		if err := icloud.SaveKeychainPassword(appleID, body.Password); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		label = appleID
	}
	if label == "" {
		label = spec.Label
	}

	acct, err := h.accounts.Create(meta.SourceAccount{
		Vendor: body.Vendor, Region: body.Region, Label: label,
	}, !spec.MultiAccount)
	if err != nil {
		// Single-account violation and other guard failures surface as 400.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

// HandleAccountItem serves DELETE /api/sources/accounts/{id}.
func (h *Handler) HandleAccountItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/sources/accounts/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "account id required", http.StatusBadRequest)
		return
	}
	acct, ok, err := h.accounts.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	// iCloud: also drop the Keychain secret (best-effort). Bronze rows are left
	// in place (see SeedLegacyAccounts note).
	if acct.Vendor == meta.VendorICloud {
		_ = icloud.DeleteKeychainPassword(acct.Label)
	}
	if err := h.accounts.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
