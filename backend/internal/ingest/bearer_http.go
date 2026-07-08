package ingest

import (
	"encoding/json"
	"net/http"

	"github.com/scottzx/1Agents/backend/internal/sources"
)

// HandleBearer serves /api/sources/{source}/bearer for manifest REST sources whose
// authKind is "bearer".
//
//	GET  ?accountId= → {configured bool}
//	PUT  {accountId?, token} → store the token (empty token clears it)
func (h *Handler) HandleBearer(w http.ResponseWriter, r *http.Request) {
	source := sourceFromPath(r.URL.Path)
	if source == "" {
		http.Error(w, "source required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		accountID := h.bearerAccount(source, r.URL.Query().Get("accountId"))
		writeJSON(w, http.StatusOK, map[string]any{"configured": sources.BearerConfigured(source, accountID)})
	case http.MethodPut:
		var body struct {
			AccountID string `json:"accountId"`
			Token     string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		accountID := h.bearerAccount(source, body.AccountID)
		if body.Token == "" {
			sources.DeleteBearerToken(source, accountID)
			writeJSON(w, http.StatusOK, map[string]any{"configured": false})
			return
		}
		if err := sources.SaveBearerToken(source, accountID, body.Token); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"configured": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// bearerAccount resolves the account id: an explicit request value, else the
// source's first registered account (or "default"). The token store is keyed by it.
func (h *Handler) bearerAccount(source, requested string) string {
	if requested != "" {
		return requested
	}
	return h.manifestAccountID(source)
}
