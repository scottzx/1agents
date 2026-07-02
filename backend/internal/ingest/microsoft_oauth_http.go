package ingest

// microsoft_oauth_http.go serves the Microsoft Graph connect flow (real OAuth
// authorization-code + PKCE, region-aware for 大陆/21Vianet). The account is
// registered first (POST /api/sources/accounts, an OAuth placeholder); this flow
// then attaches a token to it:
//
//	POST /api/sources/oauth/microsoft/start    {accountId} → {authUrl}
//	GET  /api/sources/oauth/microsoft/callback ?code&state → stores token, HTML page
//	GET  /api/sources/oauth/microsoft/status   ?accountId  → {configured,connected,…}
//	POST /api/sources/oauth/microsoft/disconnect {accountId} → drops the token
//
// PKCE verifiers are held in memory keyed by the CSRF state until the callback
// returns; they expire so an abandoned connect can't linger.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// pendingAuth is one in-flight connect (verifier + which account/region), kept
// until the callback consumes it.
type pendingAuth struct {
	accountID string
	region    string
	verifier  string
	createdAt time.Time
}

// msAuthPending is the process-wide store of in-flight connects, keyed by state.
var msAuthPending = struct {
	sync.Mutex
	m map[string]pendingAuth
}{m: map[string]pendingAuth{}}

const msAuthTTL = 10 * time.Minute

func putPending(state string, p pendingAuth) {
	msAuthPending.Lock()
	defer msAuthPending.Unlock()
	// Opportunistic GC of expired entries.
	for k, v := range msAuthPending.m {
		if time.Since(v.createdAt) > msAuthTTL {
			delete(msAuthPending.m, k)
		}
	}
	msAuthPending.m[state] = p
}

func takePending(state string) (pendingAuth, bool) {
	msAuthPending.Lock()
	defer msAuthPending.Unlock()
	p, ok := msAuthPending.m[state]
	if ok {
		delete(msAuthPending.m, state)
	}
	if ok && time.Since(p.createdAt) > msAuthTTL {
		return pendingAuth{}, false
	}
	return p, ok
}

// HandleMSOAuthStart serves POST /api/sources/oauth/microsoft/start. It builds
// the region-correct authorization URL (大陆 vs 国际) for the account and returns
// it for the client to open.
func (h *Handler) HandleMSOAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AccountID string `json:"accountId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	acct, ok, err := h.accounts.Get(strings.TrimSpace(body.AccountID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	if acct.Vendor != meta.VendorMicrosoft {
		http.Error(w, "account is not a Microsoft source", http.StatusBadRequest)
		return
	}
	if !h.msAuth.Configured(acct.Region) {
		http.Error(w, "microsoft OAuth not configured for region "+acct.Region+" (see ~/.1agents/sources/microsoft_oauth.json)", http.StatusPreconditionFailed)
		return
	}
	verifier, state := sources.NewPKCE()
	authURL, err := h.msAuth.AuthURL(acct.Region, state, verifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	putPending(state, pendingAuth{accountID: acct.ID, region: acct.Region, verifier: verifier, createdAt: time.Now()})
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": authURL})
}

// HandleMSOAuthCallback serves GET /api/sources/oauth/microsoft/callback — the
// redirect target registered on the Azure app. It exchanges the code (with the
// pending PKCE verifier), persists the token, and updates the account label to
// the connected mailbox. Responds with a small self-closing HTML page.
func (h *Handler) HandleMSOAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		msAuthResultPage(w, false, "授权失败: "+e+" "+q.Get("error_description"))
		return
	}
	state := q.Get("state")
	code := q.Get("code")
	if state == "" || code == "" {
		msAuthResultPage(w, false, "缺少 code/state 参数")
		return
	}
	pend, ok := takePending(state)
	if !ok {
		msAuthResultPage(w, false, "授权会话已过期或无效,请重试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tok, err := h.msAuth.Exchange(ctx, pend.region, code, pend.verifier)
	if err != nil {
		msAuthResultPage(w, false, err.Error())
		return
	}
	if err := h.msAuth.SaveToken(pend.accountID, tok); err != nil {
		msAuthResultPage(w, false, "保存令牌失败: "+err.Error())
		return
	}
	// Best-effort: label the account with the connected mailbox.
	if email := h.msAuth.UserEmail(ctx, pend.region, tok.AccessToken); email != "" {
		_ = h.accounts.SetLabel(pend.accountID, email)
	}
	msAuthResultPage(w, true, "Microsoft 账号已连接,可关闭本页面返回应用。")
}

// HandleMSOAuthStatus serves GET /api/sources/oauth/microsoft/status?accountId=.
func (h *Handler) HandleMSOAuthStatus(w http.ResponseWriter, r *http.Request) {
	accountID := strings.TrimSpace(r.URL.Query().Get("accountId"))
	acct, ok, err := h.accounts.Get(accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	connected, expiresAt, scope := h.msAuth.Status(accountID)
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": h.msAuth.Configured(acct.Region),
		"connected":  connected,
		"expiresAt":  expiresAt,
		"scope":      scope,
		"region":     acct.Region,
	})
}

// HandleMSOAuthDisconnect serves POST /api/sources/oauth/microsoft/disconnect.
func (h *Handler) HandleMSOAuthDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AccountID string `json:"accountId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	h.msAuth.DeleteToken(strings.TrimSpace(body.AccountID))
	w.WriteHeader(http.StatusNoContent)
}

// msAuthResultPage writes the callback's terminal HTML.
func msAuthResultPage(w http.ResponseWriter, okState bool, msg string) {
	icon, color := "✅", "#16a34a"
	if !okState {
		icon, color = "⚠️", "#dc2626"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html lang="zh"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>Microsoft 授权</title></head>` +
		`<body style="font-family:system-ui,-apple-system,'Segoe UI',sans-serif;display:flex;` +
		`align-items:center;justify-content:center;min-height:100vh;margin:0;background:#f8fafc;color:#0f172a">` +
		`<div style="text-align:center;max-width:32rem;padding:2rem">` +
		`<div style="font-size:3rem">` + icon + `</div>` +
		`<p style="font-size:1.05rem;line-height:1.6;color:` + color + `">` + htmlEscape(msg) + `</p>` +
		`<p style="color:#64748b;font-size:.85rem">本页面可安全关闭。</p>` +
		`</div><script>setTimeout(function(){try{window.close()}catch(e){}},2500)</script></body></html>`))
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
