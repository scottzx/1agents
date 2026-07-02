package sources

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSetRegionConfigHotUpdateNoDeadlock(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	a := &MSAuth{}
	if a.Configured(RegionCN) {
		t.Fatal("should start unconfigured")
	}
	// SetRegionConfig holds the write lock; it must not re-enter a read lock.
	done := make(chan error, 1)
	go func() { done <- a.SetRegionConfig(RegionCN, "cid", "common", "http://localhost:38091/cb") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SetRegionConfig: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SetRegionConfig deadlocked")
	}
	if !a.Configured(RegionCN) {
		t.Fatal("should be configured after set (hot update)")
	}
	rc := a.RegionConfig(RegionCN)
	if rc.ClientID != "cid" || rc.RedirectURI != "http://localhost:38091/cb" {
		t.Fatalf("config not applied: %+v", rc)
	}
	// A fresh loader over the same home must see the persisted values.
	cfg, err := loadMSOAuthConfig()
	if err != nil || cfg.CN.ClientID != "cid" {
		t.Fatalf("config not persisted: %+v err=%v", cfg, err)
	}
}

func TestMSEndpointsPerRegion(t *testing.T) {
	cn := msEndpointsFor(RegionCN)
	if !strings.Contains(cn.authorizeTmpl, "login.partner.microsoftonline.cn") {
		t.Fatalf("大陆 authority should be 21Vianet, got %q", cn.authorizeTmpl)
	}
	if cn.graphBase != graphBaseCN {
		t.Fatalf("大陆 graph base = %q; want %q", cn.graphBase, graphBaseCN)
	}
	intl := msEndpointsFor(RegionIntl)
	if !strings.Contains(intl.authorizeTmpl, "login.microsoftonline.com") || intl.graphBase != graphBaseIntl {
		t.Fatalf("国际 endpoints wrong: %+v", intl)
	}
}

func TestMSScopesAreRegionQualified(t *testing.T) {
	for _, s := range msScopes(RegionCN) {
		if strings.HasPrefix(s, "https://") && !strings.Contains(s, "microsoftgraph.chinacloudapi.cn") {
			t.Fatalf("大陆 graph scope should target the CN host, got %q", s)
		}
	}
	// reserved scopes carry no resource prefix
	joined := strings.Join(msScopes(RegionIntl), " ")
	if !strings.Contains(joined, "offline_access") {
		t.Fatalf("offline_access (refresh token) must be requested: %q", joined)
	}
	if !strings.Contains(joined, "https://graph.microsoft.com/Contacts.Read") {
		t.Fatalf("国际 contacts scope missing: %q", joined)
	}
}

func TestPKCEChallengeIsS256(t *testing.T) {
	v := "test-verifier-abc123"
	want := base64.RawURLEncoding.EncodeToString(sha256Sum(v))
	if got := pkceChallenge(v); got != want {
		t.Fatalf("pkceChallenge = %q; want %q", got, want)
	}
}

func sha256Sum(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func TestAuthURLBuildsCNAuthorizeWithPKCE(t *testing.T) {
	a := &MSAuth{cfg: MSOAuthConfig{
		CN: MSOAuthRegionConfig{
			ClientID:    "cn-client-id",
			Tenant:      "contoso.partner.onmschina.cn",
			RedirectURI: "http://localhost:8080/api/sources/oauth/microsoft/callback",
		},
	}}
	raw, err := a.AuthURL(RegionCN, "state-nonce", "verifier-xyz")
	if err != nil {
		t.Fatalf("AuthURL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Host != "login.partner.microsoftonline.cn" {
		t.Fatalf("大陆 auth host = %q; want 21Vianet", u.Host)
	}
	if !strings.HasPrefix(u.Path, "/contoso.partner.onmschina.cn/") {
		t.Fatalf("tenant not in path: %q", u.Path)
	}
	q := u.Query()
	if q.Get("client_id") != "cn-client-id" {
		t.Fatalf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("PKCE method = %q; want S256", q.Get("code_challenge_method"))
	}
	if q.Get("code_challenge") != pkceChallenge("verifier-xyz") {
		t.Fatalf("code_challenge mismatch")
	}
	if q.Get("state") != "state-nonce" {
		t.Fatalf("state = %q", q.Get("state"))
	}
	if !strings.Contains(q.Get("scope"), "microsoftgraph.chinacloudapi.cn/Contacts.Read") {
		t.Fatalf("scope not CN-qualified: %q", q.Get("scope"))
	}
}

func TestAuthURLUnconfiguredRegionErrors(t *testing.T) {
	a := &MSAuth{}
	if _, err := a.AuthURL(RegionCN, "s", "v"); err == nil {
		t.Fatalf("expected error for unconfigured region")
	}
}

func TestMapGraphRecordTombstone(t *testing.T) {
	rec, ok := mapGraphRecord("ms_contact", "ms_contact", []byte(`{"id":"abc","@removed":{"reason":"deleted"}}`))
	if !ok || !rec.Deleted || rec.UID != "abc" {
		t.Fatalf("tombstone mapping wrong: %+v ok=%v", rec, ok)
	}
	rec2, ok := mapGraphRecord("ms_contact", "ms_contact", []byte(`{"id":"x1","@odata.etag":"W/\"1\"","displayName":"Li"}`))
	if !ok || rec2.Deleted || rec2.ETag != `W/"1"` {
		t.Fatalf("live record mapping wrong: %+v ok=%v", rec2, ok)
	}
	if _, ok := mapGraphRecord("ms_contact", "ms_contact", []byte(`{"noid":true}`)); ok {
		t.Fatalf("record without id should be skipped")
	}
}
