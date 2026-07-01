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

	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// errNoICloudCreds is returned when a sync is attempted before credentials are set.
var errNoICloudCreds = errors.New("icloud credentials not configured")

// SyncICloudContacts pulls the iCloud address book via CardDAV into the contacts
// table as degree-1 contacts (keyed on the first phone). Returns created/updated.
func (h *Handler) SyncICloudContacts() (created, updated int, err error) {
	if err := h.requireConsent(ModICloudContacts); err != nil {
		return 0, 0, err
	}
	appleID, password, ok, err := icloud.LoadCredentials()
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, errNoICloudCreds
	}
	people, err := icloud.NewClient(appleID, password).FetchContacts()
	if err != nil {
		return 0, 0, err
	}
	imported := make([]meta.ImportedContact, 0, len(people))
	for _, p := range people {
		phone := ""
		if len(p.Phones) > 0 {
			phone = p.Phones[0]
		}
		imported = append(imported, meta.ImportedContact{
			Phone: phone, Name: p.Name, Company: p.Org, Title: p.Title,
		})
	}
	return h.cs.IngestContacts(imported)
}

// HandleICloudCredentials manages the stored iCloud credential:
//
//	GET    → {configured, appleId}      (never returns the password)
//	POST   {appleId, password}          (store; password goes to the Keychain)
//	DELETE → clear
func (h *Handler) HandleICloudCredentials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id, configured := icloud.Status()
		writeJSON(w, http.StatusOK, map[string]any{"configured": configured, "appleId": id})
	case http.MethodPost:
		var body struct {
			AppleID  string `json:"appleId"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
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

// HandleICloudSync: POST /api/contacts/icloud/sync → pull the iCloud address book.
func (h *Handler) HandleICloudSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	created, updated, err := h.SyncICloudContacts()
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
