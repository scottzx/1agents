// icloud.go adds the iCloud contacts channel via CardDAV: the user supplies their
// Apple ID + an app-specific password (a delegated authorization to their own
// data), stored locally (Keychain), and the pull runs locally — data never
// transits our servers. This replaces the earlier local CNContactStore path and
// removes the macOS Contacts-permission dependency. Calendars/mail over the same
// credential (CalDAV/IMAP) are natural follow-ups.
package contacts

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/scottzx/1Agents/backend/internal/govern"
	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// errNoICloudCreds is returned when a sync is attempted before credentials are set.
var errNoICloudCreds = errors.New("icloud credentials not configured")

// SyncICloudContacts refreshes iCloud contacts in two decoupled steps: pull the
// address books over CardDAV into the raw bronze store (incremental — unchanged
// books are skipped by CTag and only changed vCards are fetched, so we stop
// re-downloading everything and tripping iCloud's throttling), then govern
// bronze → the gold contacts table (offline, re-runnable). Returns the
// created/updated gold counts. A ThrottledError from the pull propagates
// unwrapped so HandleICloudSync can map it to 503 + Retry-After.
func (h *Handler) SyncICloudContacts(accountID string) (created, updated int, err error) {
	if err := h.requireConsent(ModICloudContacts); err != nil {
		return 0, 0, err
	}
	st, err := sources.OpenDefault()
	if err != nil {
		return 0, 0, err
	}
	pullers, err := h.icloudPullers(accountID)
	if err != nil {
		return 0, 0, err
	}
	if len(pullers) == 0 {
		return 0, 0, errNoICloudCreds
	}
	// Pull each account's address books into bronze under its own account_id, then
	// govern bronze → gold once (governance merges every iCloud account's contacts
	// into the unified address book).
	for _, p := range pullers {
		if _, err := st.Sync(p.puller, p.accountID); err != nil {
			return 0, 0, err
		}
	}
	return govern.ICloudContacts(st, h.cs)
}

// icloudPull binds a puller to the bronze account_id it writes under.
type icloudPull struct {
	accountID string
	puller    sources.Puller
}

// icloudPullers resolves the iCloud puller(s) to run: one account when accountID
// is set, else every registered iCloud account. When the account registry is
// unavailable (tests) it falls back to the legacy single credential under the
// "default" account_id. Each account's password comes from the Keychain (keyed
// by Apple ID) and its discovery root from its region (国际/大陆).
func (h *Handler) icloudPullers(accountID string) ([]icloudPull, error) {
	if h.sas == nil {
		appleID, password, ok, err := icloud.LoadCredentials()
		if err != nil || !ok {
			return nil, err
		}
		return []icloudPull{{accountID: "default", puller: sources.NewICloudPuller(appleID, password)}}, nil
	}
	var accts []meta.SourceAccount
	if accountID != "" {
		a, ok, err := h.sas.Get(accountID)
		if err != nil {
			return nil, err
		}
		if ok && a.Vendor == meta.VendorICloud {
			accts = []meta.SourceAccount{a}
		}
	} else {
		list, err := h.sas.ListByVendor(meta.VendorICloud)
		if err != nil {
			return nil, err
		}
		accts = list
	}
	out := make([]icloudPull, 0, len(accts))
	for _, a := range accts {
		password, ok, err := icloud.LoadKeychainPassword(a.Label)
		if err != nil || !ok {
			continue // no stored credential for this account — skip it
		}
		out = append(out, icloudPull{
			accountID: a.ID,
			puller:    sources.NewICloudPullerRegion(a.Region, a.Label, password),
		})
	}
	return out, nil
}

// HandleICloudCredentials manages the stored iCloud credential:
//
//	GET    → {configured, appleId}      (never returns the password)
//	POST   {appleId, password}          (store; password goes to the Keychain)
//	DELETE → clear
func (h *Handler) HandleICloudCredentials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// accountId (query) scopes the status to one 数据源 account: configured means
		// its Apple ID has a Keychain password. No accountId → legacy single cred.
		if accountID := r.URL.Query().Get("accountId"); accountID != "" && h.sas != nil {
			a, ok, err := h.sas.Get(accountID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			configured := false
			if ok {
				_, configured, _ = icloud.LoadKeychainPassword(a.Label)
			}
			appleID := ""
			if ok {
				appleID = a.Label
			}
			writeJSON(w, http.StatusOK, map[string]any{"configured": configured, "appleId": appleID})
			return
		}
		id, configured := icloud.Status()
		writeJSON(w, http.StatusOK, map[string]any{"configured": configured, "appleId": id})
	case http.MethodPost:
		var body struct {
			AccountID string `json:"accountId"`
			AppleID   string `json:"appleId"`
			Password  string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		// Account-scoped (源为中心): re-enter the password for an existing account —
		// the Apple ID comes from the account, the secret goes to the Keychain.
		if body.AccountID != "" && h.sas != nil {
			a, ok, err := h.sas.Get(body.AccountID)
			if err != nil || !ok {
				http.Error(w, "account not found", http.StatusNotFound)
				return
			}
			if err := icloud.SaveKeychainPassword(a.Label, body.Password); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"configured": true, "appleId": a.Label})
			return
		}
		if err := icloud.SaveCredentials(body.AppleID, body.Password); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "appleId": body.AppleID})
	case http.MethodDelete:
		if err := icloud.ClearCredentials(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleICloudSync: POST /api/contacts/icloud/sync → pull the iCloud address
// book. Body {"accountId":"…"} scopes the pull to one 数据源 account (源为中心);
// an empty/absent accountId syncs every registered iCloud account.
func (h *Handler) HandleICloudSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AccountID string `json:"accountId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // optional body
	created, updated, err := h.SyncICloudContacts(body.AccountID)
	if errors.Is(err, errNoICloudCreds) || errors.Is(err, errNotConsented) {
		http.Error(w, err.Error(), http.StatusPreconditionRequired)
		return
	}
	var throttled *icloud.ThrottledError
	if errors.As(err, &throttled) {
		// iCloud is back-pressuring (ck throttling). Tell the client to retry later
		// rather than reporting it as a gateway failure.
		if throttled.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(throttled.RetryAfter.Seconds())))
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "updated": updated})
}
