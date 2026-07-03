package sources

// microsoft_oauth.go implements the real Microsoft (Entra ID) authorization-code
// flow with PKCE for the Graph data source. It is region-aware: the 大陆
// (21Vianet / 世纪互联) cloud is a physically separate sovereign cloud, so both
// the identity authority (login.partner.microsoftonline.cn) and the Graph
// resource (microsoftgraph.chinacloudapi.cn) differ from 国际. A global app
// registration is rejected by the CN authority and vice-versa — the client
// config is therefore per-region (config file, one block each).
//
// Client type: public client + PKCE, so no client_secret is stored (a secret is
// still honored if the config carries one, for Web-type registrations). Tokens
// live in a per-account file under ~/.1agents/sources/ms_tokens (mode 0600),
// keyed by the bronze account_id — naturally multi-account, same rationale as
// the iCloud Keychain store.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Graph base endpoints per region (also consumed by microsoft_puller.go).
const (
	graphBaseIntl = "https://graph.microsoft.com/v1.0"
	graphBaseCN   = "https://microsoftgraph.chinacloudapi.cn/v1.0"
)

// msDelegatedScopes are the delegated permissions requested for the connect. The
// resource is region-qualified at request time (msScopes) because a sovereign
// cloud will only mint a token whose audience is its own Graph host. openid /
// offline_access are reserved scopes (no resource prefix); offline_access is
// what yields the refresh token that keeps the connection alive.
var msDelegatedScopes = []string{"User.Read", "Contacts.Read", "Mail.Read", "Calendars.Read", "Tasks.Read"}

// MSOAuthRegionConfig is one region's app registration. ClientSecret is optional
// (empty ⇒ public client + PKCE only). Tenant is "common" / "organizations" /
// "consumers" / a tenant GUID or verified domain; CN tenants are single-tenant,
// so the tenant id/domain is the norm there.
type MSOAuthRegionConfig struct {
	ClientID     string `json:"clientId"`
	Tenant       string `json:"tenant"`
	RedirectURI  string `json:"redirectUri"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

func (c MSOAuthRegionConfig) configured() bool {
	return strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.RedirectURI) != ""
}

func (c MSOAuthRegionConfig) tenant() string {
	if t := strings.TrimSpace(c.Tenant); t != "" {
		return t
	}
	return "common"
}

// MSOAuthConfig is the two-region client config, loaded from
// ~/.1agents/sources/microsoft_oauth.json.
type MSOAuthConfig struct {
	Intl MSOAuthRegionConfig `json:"intl"`
	CN   MSOAuthRegionConfig `json:"cn"`
}

func (c MSOAuthConfig) forRegion(region string) MSOAuthRegionConfig {
	if region == RegionCN {
		return c.CN
	}
	return c.Intl
}

// msEndpoints bundles the region's authority + Graph host.
type msEndpoints struct {
	authorizeTmpl string // fmt with tenant
	tokenTmpl     string // fmt with tenant
	graphBase     string
}

func msEndpointsFor(region string) msEndpoints {
	if region == RegionCN {
		return msEndpoints{
			authorizeTmpl: "https://login.partner.microsoftonline.cn/%s/oauth2/v2.0/authorize",
			tokenTmpl:     "https://login.partner.microsoftonline.cn/%s/oauth2/v2.0/token",
			graphBase:     graphBaseCN,
		}
	}
	return msEndpoints{
		authorizeTmpl: "https://login.microsoftonline.com/%s/oauth2/v2.0/authorize",
		tokenTmpl:     "https://login.microsoftonline.com/%s/oauth2/v2.0/token",
		graphBase:     graphBaseIntl,
	}
}

// graphHostFor returns the resource host (no /v1.0) used to qualify Graph scopes
// for the region's sovereign cloud.
func graphHostFor(region string) string {
	return strings.TrimSuffix(msEndpointsFor(region).graphBase, "/v1.0")
}

// msScopes returns the space-join-ready scope list for a region: reserved scopes
// plus resource-qualified Graph permissions (audience must match the region's
// Graph host or the sovereign authority refuses the token).
func msScopes(region string) []string {
	host := graphHostFor(region)
	out := []string{"offline_access", "openid", "profile"}
	for _, s := range msDelegatedScopes {
		out = append(out, host+"/"+s)
	}
	return out
}

// StoredToken is one account's persisted OAuth material. ExpiresAt is epoch
// seconds; Region pins which authority/Graph host to talk to on refresh.
type StoredToken struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
	Scope        string `json:"scope"`
	Region       string `json:"region"`
}

func (t StoredToken) expired() bool {
	// 60s skew so a token that dies mid-request is refreshed proactively.
	return time.Now().Unix() >= t.ExpiresAt-60
}

// msTokenResp is the raw token-endpoint response.
type msTokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// MSAuth owns the Microsoft OAuth flow: config, PKCE authorization URLs, the
// code→token exchange, refresh, and the per-account token store. It doubles as
// the puller's MSTokenProvider (AccessToken).
type MSAuth struct {
	mu   sync.RWMutex // guards cfg (hot-updated via SetRegionConfig from the UI)
	cfg  MSOAuthConfig
	dir  string // token store dir
	http *http.Client
}

// NewMSAuth loads the client config (a missing file is not an error — the flow
// simply reports "not configured" until the config is set) and prepares the
// token store dir.
func NewMSAuth() (*MSAuth, error) {
	cfg, err := loadMSOAuthConfig()
	if err != nil {
		return nil, err
	}
	return &MSAuth{
		cfg:  cfg,
		dir:  filepath.Join(sourcesHome(), "ms_tokens"),
		http: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// regionCfg returns a copy of a region's client config under the read lock.
func (a *MSAuth) regionCfg(region string) MSOAuthRegionConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.forRegion(region)
}

// RegionConfig returns a region's current client config (for the settings form
// to prefill). The clientId is an app identifier, not a secret, so it is safe to
// surface; no client_secret is ever returned.
func (a *MSAuth) RegionConfig(region string) MSOAuthRegionConfig {
	rc := a.regionCfg(region)
	rc.ClientSecret = ""
	return rc
}

// SetRegionConfig updates a region's app registration (clientId/tenant, and
// redirectUri when non-empty) both in memory (hot — no restart) and on disk.
// This backs the in-UI "configure & connect" form so the user never hand-edits
// the JSON.
func (a *MSAuth) SetRegionConfig(region, clientID, tenant, redirectURI string) error {
	a.mu.Lock()
	rc := a.cfg.forRegion(region) // already holding the write lock — read directly

	rc.ClientID = strings.TrimSpace(clientID)
	rc.Tenant = strings.TrimSpace(tenant)
	if strings.TrimSpace(redirectURI) != "" {
		rc.RedirectURI = strings.TrimSpace(redirectURI)
	}
	if region == RegionCN {
		a.cfg.CN = rc
	} else {
		a.cfg.Intl = rc
	}
	cfg := a.cfg
	a.mu.Unlock()
	return saveMSOAuthConfig(cfg)
}

// Configured reports whether the region has a usable app registration.
func (a *MSAuth) Configured(region string) bool { return a.regionCfg(region).configured() }

// AuthURL builds the authorization-code request for a region. verifier is the
// PKCE code verifier (NewPKCE); its S256 challenge is embedded. state is echoed
// back to the callback to match the pending request.
func (a *MSAuth) AuthURL(region, state, verifier string) (string, error) {
	rc := a.regionCfg(region)
	if !rc.configured() {
		return "", fmt.Errorf("microsoft: region %q not configured (see microsoft_oauth.json)", region)
	}
	ep := msEndpointsFor(region)
	q := url.Values{}
	q.Set("client_id", rc.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", rc.RedirectURI)
	q.Set("response_mode", "query")
	q.Set("scope", strings.Join(msScopes(region), " "))
	q.Set("state", state)
	q.Set("code_challenge", pkceChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	// prompt=select_account so re-connecting can pick a different mailbox.
	q.Set("prompt", "select_account")
	return fmt.Sprintf(ep.authorizeTmpl, rc.tenant()) + "?" + q.Encode(), nil
}

// Exchange trades an authorization code (+ PKCE verifier) for tokens.
func (a *MSAuth) Exchange(ctx context.Context, region, code, verifier string) (StoredToken, error) {
	rc := a.regionCfg(region)
	form := url.Values{}
	form.Set("client_id", rc.ClientID)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", rc.RedirectURI)
	form.Set("code_verifier", verifier)
	form.Set("scope", strings.Join(msScopes(region), " "))
	if rc.ClientSecret != "" {
		form.Set("client_secret", rc.ClientSecret)
	}
	return a.postToken(ctx, region, form)
}

// AccessToken returns a fresh access token for an account, refreshing (and
// persisting) when the stored token is expired. Implements MSTokenProvider.
func (a *MSAuth) AccessToken(accountID string) (string, error) {
	tok, ok, err := a.LoadToken(accountID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("microsoft: account %s not connected", accountID)
	}
	if !tok.expired() {
		return tok.AccessToken, nil
	}
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("microsoft: account %s token expired and no refresh token", accountID)
	}
	refreshed, err := a.refresh(context.Background(), tok)
	if err != nil {
		return "", err
	}
	if err := a.SaveToken(accountID, refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func (a *MSAuth) refresh(ctx context.Context, tok StoredToken) (StoredToken, error) {
	rc := a.regionCfg(tok.Region)
	form := url.Values{}
	form.Set("client_id", rc.ClientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", tok.RefreshToken)
	// Refresh with the ALREADY-GRANTED scope, not the current full wish-list:
	// asking for a scope the user hasn't consented to (e.g. Calendars.Read added
	// after this token was minted) makes the refresh fail. New scopes are granted
	// only by an interactive re-connect (AuthURL uses the full msScopes).
	scope := tok.Scope
	if scope == "" {
		scope = strings.Join(msScopes(tok.Region), " ")
	}
	form.Set("scope", scope)
	if rc.ClientSecret != "" {
		form.Set("client_secret", rc.ClientSecret)
	}
	next, err := a.postToken(ctx, tok.Region, form)
	if err != nil {
		return StoredToken{}, err
	}
	// A refresh may omit a new refresh token — keep the prior one.
	if next.RefreshToken == "" {
		next.RefreshToken = tok.RefreshToken
	}
	return next, nil
}

// postToken POSTs a token-endpoint form and maps the response to a StoredToken.
func (a *MSAuth) postToken(ctx context.Context, region string, form url.Values) (StoredToken, error) {
	ep := msEndpointsFor(region)
	rc := a.regionCfg(region)
	endpoint := fmt.Sprintf(ep.tokenTmpl, rc.tenant())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return StoredToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return StoredToken{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr msTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return StoredToken{}, fmt.Errorf("microsoft: bad token response (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if tr.Error != "" || resp.StatusCode >= 400 {
		msg := tr.ErrorDesc
		if msg == "" {
			msg = tr.Error
		}
		return StoredToken{}, fmt.Errorf("microsoft: token endpoint: %s", strings.TrimSpace(msg))
	}
	return StoredToken{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Unix() + tr.ExpiresIn,
		Scope:        tr.Scope,
		Region:       region,
	}, nil
}

// UserEmail fetches the signed-in identity (userPrincipalName / mail) so the
// account label can reflect the connected mailbox. Best-effort.
func (a *MSAuth) UserEmail(ctx context.Context, region, accessToken string) string {
	ep := msEndpointsFor(region)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.graphBase+"/me?$select=userPrincipalName,mail,displayName", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	var me struct {
		UPN         string `json:"userPrincipalName"`
		Mail        string `json:"mail"`
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&me); err != nil {
		return ""
	}
	switch {
	case me.Mail != "":
		return me.Mail
	case me.UPN != "":
		return me.UPN
	default:
		return me.DisplayName
	}
}

// --- token store (per-account file) ---------------------------------------

func (a *MSAuth) tokenPath(accountID string) string {
	return filepath.Join(a.dir, accountID+".json")
}

// LoadToken returns an account's stored token; ok=false when never connected.
func (a *MSAuth) LoadToken(accountID string) (StoredToken, bool, error) {
	b, err := os.ReadFile(a.tokenPath(accountID))
	if os.IsNotExist(err) {
		return StoredToken{}, false, nil
	}
	if err != nil {
		return StoredToken{}, false, err
	}
	var t StoredToken
	if err := json.Unmarshal(b, &t); err != nil {
		return StoredToken{}, false, err
	}
	return t, true, nil
}

// SaveToken persists an account's token (0600, dir 0700).
func (a *MSAuth) SaveToken(accountID string, tok StoredToken) error {
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.tokenPath(accountID), b, 0o600)
}

// DeleteToken removes an account's token (best-effort; called on disconnect).
func (a *MSAuth) DeleteToken(accountID string) {
	_ = os.Remove(a.tokenPath(accountID))
}

// Status summarizes an account's connection for the UI.
func (a *MSAuth) Status(accountID string) (connected bool, expiresAt int64, scope string) {
	tok, ok, err := a.LoadToken(accountID)
	if err != nil || !ok {
		return false, 0, ""
	}
	return true, tok.ExpiresAt, tok.Scope
}

// --- config + PKCE helpers -------------------------------------------------

// sourcesHome returns ~/.1agents/sources (honoring ONEAGENTS_HOME), the sibling
// dir of meta.db / sync.db that holds source-level config + secrets.
func sourcesHome() string {
	base := os.Getenv("ONEAGENTS_HOME")
	if base == "" {
		if h, err := os.UserHomeDir(); err == nil {
			base = h
		} else {
			base = "."
		}
	}
	return filepath.Join(base, ".1agents", "sources")
}

func msOAuthConfigPath() string { return filepath.Join(sourcesHome(), "microsoft_oauth.json") }

// saveMSOAuthConfig persists the client config (0600, dir 0700). Written whole
// on every SetRegionConfig so the on-disk file always mirrors the live config.
func saveMSOAuthConfig(c MSOAuthConfig) error {
	dir := sourcesHome()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(msOAuthConfigPath(), b, 0o600)
}

func loadMSOAuthConfig() (MSOAuthConfig, error) {
	b, err := os.ReadFile(msOAuthConfigPath())
	if os.IsNotExist(err) {
		return MSOAuthConfig{}, nil
	}
	if err != nil {
		return MSOAuthConfig{}, err
	}
	var c MSOAuthConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return MSOAuthConfig{}, fmt.Errorf("microsoft: parse %s: %w", msOAuthConfigPath(), err)
	}
	return c, nil
}

// NewPKCE returns a fresh (verifier, state) pair. The verifier is a 43-char
// base64url secret; state is a shorter random nonce for CSRF matching.
func NewPKCE() (verifier, state string) {
	return randB64(32), randB64(16)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randB64(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is fatal-adjacent; fall back to a time-seeded value
		// rather than panicking in an HTTP handler.
		return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
