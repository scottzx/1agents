package system

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/nacl/box"
)

// Account-level device pairing — the "requester" side of happy's auth handshake,
// implemented entirely in Go (the agent front door owns credential bootstrap; we
// steer the happy-cli submodule via env, never patch it). This replaces the old
// "device profile" pairing (machine borrows its own token) with Model A account
// binding: the machine joins the client's account C, so C's user-scoped relay
// connection naturally carries C's subscription.
//
// Flow:
//  1. POST /api/system/happy/pair/start → generate an ephemeral box keypair,
//     register it via the relay's /v1/auth/request, and return a
//     `happy://terminal?<key>` pairing code. A background poller waits for the
//     client (account C) to approve via /v1/auth/response.
//  2. On approval the relay returns { token, response }; we box-open `response`
//     to recover account C's content public key, write dataKey credentials to
//     ~/.happy/access.key, and restart the daemon so it reconnects AS account C.
//  3. GET /api/system/happy/pair/status reports progress for the UI.

type pairStatus string

const (
	pairIdle       pairStatus = "idle"
	pairPending    pairStatus = "pending"
	pairAuthorized pairStatus = "authorized"
	pairError      pairStatus = "error"
)

const pairTimeout = 5 * time.Minute

type pairState struct {
	mu     sync.Mutex
	status pairStatus
	url    string // happy://terminal?<key>
	errMsg string
	cancel chan struct{} // closed to supersede the active poller
}

var pairing = &pairState{status: pairIdle}

// pairHTTP talks to the relay's /v1/auth/* endpoints. When the relay runs on a
// self-signed dev cert (mkcert), macOS's platform verifier won't trust it, so we
// build an explicit RootCAs pool (system roots + an extra PEM) — setting RootCAs
// makes Go use the pure-Go verifier and honor the extra CA. The extra CA path is
// read from HAPPY_EXTRA_CA_CERTS, falling back to NODE_EXTRA_CA_CERTS (which
// dev-backend.sh already exports for the spawned happy daemon). No env → default
// behaviour (system trust only), so production with real certs is unaffected.
var pairHTTP = newPairHTTPClient()

func newPairHTTPClient() *http.Client {
	client := &http.Client{Timeout: 15 * time.Second}
	caPath := os.Getenv("HAPPY_EXTRA_CA_CERTS")
	if caPath == "" {
		caPath = os.Getenv("NODE_EXTRA_CA_CERTS")
	}
	if caPath == "" {
		return client
	}
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return client
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return client
	}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	return client
}

// finish updates the terminal status, but only if this poller is still the
// active one (a newer pair/start supersedes older pollers).
func (p *pairState) finish(cancel chan struct{}, st pairStatus, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != cancel {
		return
	}
	p.status = st
	p.errMsg = errMsg
}

// HappyPairStart handles POST /api/system/happy/pair/start.
func (h *Handler) HappyPairStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		jsonError(w, "key generation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	serverURL := strings.TrimRight(effectiveRelayURL(), "/")
	pubStd := base64.StdEncoding.EncodeToString(pub[:])
	if err := authRequest(serverURL, pubStd); err != nil {
		jsonError(w, "relay auth request failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// happy://terminal?<base64url(publicKey)> — the format extractKey() / the
	// happy mobile app and our RelayPairingPanel scan.
	url := "happy://terminal?" + base64.RawURLEncoding.EncodeToString(pub[:])

	pairing.mu.Lock()
	if pairing.cancel != nil {
		close(pairing.cancel) // supersede any in-flight poller
	}
	cancel := make(chan struct{})
	pairing.status = pairPending
	pairing.url = url
	pairing.errMsg = ""
	pairing.cancel = cancel
	pairing.mu.Unlock()

	go pollPairing(serverURL, pubStd, priv, cancel)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"pairingUrl": url, "publicKey": pubStd})
}

// HappyPairStatus handles GET /api/system/happy/pair/status.
func (h *Handler) HappyPairStatus(w http.ResponseWriter, r *http.Request) {
	pairing.mu.Lock()
	out := map[string]string{"status": string(pairing.status)}
	if pairing.errMsg != "" {
		out["error"] = pairing.errMsg
	}
	if pairing.url != "" {
		out["pairingUrl"] = pairing.url
	}
	pairing.mu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

// authRequest registers the ephemeral public key with the relay so the client
// can find and approve it.
func authRequest(serverURL, pubStd string) error {
	body, _ := json.Marshal(map[string]any{"publicKey": pubStd, "supportsV2": true})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/v1/auth/request", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Happy-Client", "1agents-backend")
	resp, err := pairHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// pollPairing waits (up to pairTimeout) for account C to approve, then completes.
func pollPairing(serverURL, pubStd string, priv *[32]byte, cancel chan struct{}) {
	deadline := time.Now().Add(pairTimeout)
	for {
		select {
		case <-cancel:
			return
		default:
		}
		if time.Now().After(deadline) {
			pairing.finish(cancel, pairError, "pairing timed out — no approval within 5 minutes")
			return
		}

		token, respB64, authorized, err := authPoll(serverURL, pubStd)
		if err == nil && authorized {
			if e := completePairing(token, respB64, priv); e != nil {
				pairing.finish(cancel, pairError, e.Error())
			} else {
				pairing.finish(cancel, pairAuthorized, "")
			}
			return
		}

		select {
		case <-cancel:
			return
		case <-time.After(time.Second):
		}
	}
}

// authPoll polls /v1/auth/request; returns authorized=true with the token and
// encrypted response once the client has approved.
func authPoll(serverURL, pubStd string) (token, response string, authorized bool, err error) {
	body, _ := json.Marshal(map[string]any{"publicKey": pubStd, "supportsV2": true})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/v1/auth/request", bytes.NewReader(body))
	if err != nil {
		return "", "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Happy-Client", "1agents-backend")
	resp, err := pairHTTP.Do(req)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		State    string `json:"state"`
		Token    string `json:"token"`
		Response string `json:"response"`
	}
	if e := json.NewDecoder(resp.Body).Decode(&out); e != nil {
		return "", "", false, e
	}
	if out.State == "authorized" {
		return out.Token, out.Response, true, nil
	}
	return "", "", false, nil
}

// completePairing box-opens the client's response to recover account C's content
// public key, writes dataKey credentials, and restarts the daemon as account C.
//
// The response bundle (client's encryptForPublicKey output) is:
//
//	ephemeralPublicKey(32) | nonce(24) | box-ciphertext
//
// and the decrypted plaintext is [0x00, contentPublicKey(32)].
func completePairing(token, respB64 string, priv *[32]byte) error {
	contentPub, err := openApprovalResponse(respB64, priv)
	if err != nil {
		return err
	}

	machineKey := make([]byte, 32)
	if _, err := rand.Read(machineKey); err != nil {
		return fmt.Errorf("machine key generation failed: %w", err)
	}
	if err := writeDataKeyCredentials(token, contentPub, machineKey); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	// Rebinding to a new account: drop the old machineId so the daemon mints a
	// fresh one on restart. Machine.id is a GLOBAL primary key on the server —
	// reusing the previous account's machineId under account C fails the create
	// with P2002 (unique violation) and the machine never registers (client then
	// sees "0 nodes"). A fresh id sidesteps the collision; the stale machine on
	// the old account is orphaned and reaped.
	_ = clearHappyMachineID()

	// Restart the daemon so it reads the new credentials and reconnects as C.
	_ = stopHappyDaemonProcess()
	time.Sleep(500 * time.Millisecond)
	return startHappyDaemon()
}

// clearHappyMachineID removes machineId from ~/.happy/settings.json (preserving
// all other keys) so the daemon regenerates a fresh id on next start. Called on
// account rebind — see completePairing for why the old id must not be reused.
func clearHappyMachineID() error {
	path := filepath.Join(happyHome(), "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // no settings yet → daemon creates fresh
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	if _, ok := m["machineId"]; !ok {
		return nil
	}
	delete(m, "machineId")
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

// openApprovalResponse box-opens the client's approval bundle and returns
// account C's content public key. The bundle (client's encryptForPublicKey
// output) is ephemeralPublicKey(32) | nonce(24) | box-ciphertext, and the
// decrypted plaintext is [0x00, contentPublicKey(32)].
func openApprovalResponse(respB64 string, priv *[32]byte) ([]byte, error) {
	bundle, err := base64.StdEncoding.DecodeString(respB64)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(bundle) < 32+24+box.Overhead+1 {
		return nil, fmt.Errorf("response bundle too short (%d bytes)", len(bundle))
	}
	var ephPub [32]byte
	var nonce [24]byte
	copy(ephPub[:], bundle[0:32])
	copy(nonce[:], bundle[32:56])
	ciphertext := bundle[56:]

	plain, ok := box.Open(nil, ciphertext, &nonce, &ephPub, priv)
	if !ok {
		return nil, fmt.Errorf("failed to decrypt approval response")
	}
	if len(plain) != 33 || plain[0] != 0 {
		return nil, fmt.Errorf("unexpected approval payload (len=%d, tag=%d)", len(plain), plain[0])
	}
	return plain[1:33], nil
}

// writeDataKeyCredentials writes ~/.happy/access.key in happy-cli's dataKey
// format: { token, encryption: { publicKey, machineKey } } (matches
// persistence.ts writeCredentialsDataKey, which the daemon reads back).
func writeDataKeyCredentials(token string, contentPub, machineKey []byte) error {
	creds := map[string]any{
		"token": token,
		"encryption": map[string]string{
			"publicKey":  base64.StdEncoding.EncodeToString(contentPub),
			"machineKey": base64.StdEncoding.EncodeToString(machineKey),
		},
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(happyHome(), "access.key")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
