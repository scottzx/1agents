package system

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// HappyEnsureMachine handles POST /api/system/happy/ensure-machine.
//
// Uses ~/.1agents/relay-creds.json (user account token + secretB64) to complete
// the auth handshake against the relay without a human scanning a QR:
// auth/request → approve as the same account → write ~/.happy/access.key →
// restart daemon. Device then has a machine session under that user account
// and can expose a Model B config QR.
//
// Body (optional JSON): { "force": true } to rebind even when access.key exists
// and already matches the account.
func (h *Handler) HappyEnsureMachine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Force bool `json:"force"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)

	result, err := ensureMachineBoundToRelayAccount(body.Force)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}

type ensureResult struct {
	OK        bool   `json:"ok"`
	Skipped   bool   `json:"skipped,omitempty"`
	Reason    string `json:"reason,omitempty"`
	MachineID string `json:"machineId,omitempty"`
	ServerURL string `json:"serverUrl,omitempty"`
}

// ensureMachineBoundToRelayAccount binds this host to the account in
// relay-creds.json and starts the happy daemon when needed.
func ensureMachineBoundToRelayAccount(force bool) (*ensureResult, error) {
	creds, err := loadRelayCredentialsFile()
	if err != nil {
		return nil, fmt.Errorf("no relay account on this host: %w (save via POST /api/relay/credentials first)", err)
	}
	if creds.Token == "" || creds.SecretB64 == "" {
		return nil, fmt.Errorf("relay-creds.json missing token or secretB64")
	}

	serverURL := strings.TrimRight(creds.RelayURL, "/")
	if serverURL == "" {
		serverURL = strings.TrimRight(effectiveRelayURL(), "/")
	}

	// Already bound to the same account and not forcing → just ensure daemon.
	if !force && accessKeyMatchesAccount(creds.Token) {
		if err := ensureHappyDaemonRunning(); err != nil {
			return nil, fmt.Errorf("already bound but daemon start failed: %w", err)
		}
		return &ensureResult{
			OK:        true,
			Skipped:   true,
			Reason:    "already bound to this account",
			MachineID: happySettingsMachineID(),
			ServerURL: serverURL,
		}, nil
	}

	if err := selfApproveAndWriteCredentials(serverURL, creds.Token, creds.SecretB64); err != nil {
		return nil, err
	}
	_ = writeHappyServerURL(serverURL)

	_ = stopHappyDaemonProcess()
	time.Sleep(500 * time.Millisecond)
	if err := startHappyDaemon(); err != nil {
		return nil, fmt.Errorf("credentials written but daemon start failed: %w", err)
	}

	machineID := ""
	for i := 0; i < 20; i++ {
		time.Sleep(400 * time.Millisecond)
		machineID = happySettingsMachineID()
		if machineID != "" {
			break
		}
	}

	return &ensureResult{
		OK:        true,
		MachineID: machineID,
		ServerURL: serverURL,
	}, nil
}

func loadRelayCredentialsFile() (*RelayCredentials, error) {
	data, err := os.ReadFile(relayCredsPath())
	if err != nil {
		return nil, err
	}
	var c RelayCredentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func accessKeyMatchesAccount(accountToken string) bool {
	keyFile := filepath.Join(happyHome(), "access.key")
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return false
	}
	var key struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(data, &key) != nil || key.Token == "" {
		return false
	}
	as, ok1 := jwtSub(accountToken)
	ms, ok2 := jwtSub(key.Token)
	return ok1 && ok2 && as != "" && as == ms
}

func jwtSub(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", false
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", false
		}
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return "", false
	}
	return claims.Sub, claims.Sub != ""
}

func happySettingsMachineID() string {
	data, err := os.ReadFile(filepath.Join(happyHome(), "settings.json"))
	if err != nil {
		return ""
	}
	var s struct {
		MachineID string `json:"machineId"`
	}
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	return s.MachineID
}

func writeHappyServerURL(serverURL string) error {
	path := filepath.Join(happyHome(), "settings.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	m := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	m["serverUrl"] = serverURL
	m["webappUrl"] = serverURL
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

func ensureHappyDaemonRunning() error {
	stateFile := filepath.Join(happyHome(), "daemon.state.json")
	if data, err := os.ReadFile(stateFile); err == nil {
		var state struct {
			Pid int `json:"pid"`
		}
		if json.Unmarshal(data, &state) == nil && state.Pid > 0 {
			if proc, err := os.FindProcess(state.Pid); err == nil {
				// Signal 0: check process exists (Unix)
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return nil
				}
			}
		}
	}
	return startHappyDaemon()
}

// selfApproveAndWriteCredentials runs:
// request → approve with account secret → poll authorized → write access.key.
func selfApproveAndWriteCredentials(serverURL, accountToken, secretB64 string) error {
	secret, err := decodeSecretB64(secretB64)
	if err != nil {
		return err
	}

	contentPub, err := deriveContentPublicKey(secret)
	if err != nil {
		return fmt.Errorf("derive content key: %w", err)
	}

	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	pubStd := base64.StdEncoding.EncodeToString(pub[:])

	if err := authRequest(serverURL, pubStd); err != nil {
		return fmt.Errorf("auth/request: %w", err)
	}

	plain := make([]byte, 33)
	plain[0] = 0
	copy(plain[1:], contentPub)
	responseB64 := encryptForPublicKeyB64(plain, pub)
	if responseB64 == "" {
		return fmt.Errorf("encrypt approval response failed")
	}
	if err := authResponse(serverURL, accountToken, pubStd, responseB64); err != nil {
		return fmt.Errorf("auth/response: %w", err)
	}

	var sessionToken string
	for i := 0; i < 15; i++ {
		tok, resp, ok, err := authPoll(serverURL, pubStd)
		if err != nil {
			return fmt.Errorf("auth poll: %w", err)
		}
		if ok {
			// Sanity: decrypt response with ephemeral priv (optional).
			if _, err := openApprovalResponse(resp, priv); err != nil {
				return fmt.Errorf("decrypt approval: %w", err)
			}
			sessionToken = tok
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if sessionToken == "" {
		return fmt.Errorf("auth not authorized after self-approve")
	}

	machineKey := make([]byte, 32)
	if _, err := rand.Read(machineKey); err != nil {
		return err
	}
	_ = clearHappyMachineID()
	return writeDataKeyCredentials(sessionToken, contentPub, machineKey)
}

func decodeSecretB64(s string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
		if err != nil {
			return nil, fmt.Errorf("invalid secretB64")
		}
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("secretB64 must decode to 32 bytes, got %d", len(raw))
	}
	return raw, nil
}

// deriveContentPublicKey mirrors frontend relay/crypto.ts deriveContentKeyPair
// and returns the 32-byte box public key.
func deriveContentPublicKey(secret []byte) ([]byte, error) {
	if len(secret) != 32 {
		return nil, fmt.Errorf("secret must be 32 bytes")
	}
	seed := deriveHMACKey(secret, "Happy EnCoder", []string{"content"})
	sum := sha512.Sum512(seed)
	var sk [32]byte
	copy(sk[:], sum[:32])
	var pk [32]byte
	curve25519.ScalarBaseMult(&pk, &sk)
	return pk[:], nil
}

func deriveHMACKey(master []byte, usage string, path []string) []byte {
	mac := hmac.New(sha512.New, []byte(usage+" Master Seed"))
	mac.Write(master)
	I := mac.Sum(nil)
	key := append([]byte{}, I[:32]...)
	chainCode := append([]byte{}, I[32:64]...)
	for _, index := range path {
		data := append([]byte{0x00}, []byte(index)...)
		mac = hmac.New(sha512.New, chainCode)
		mac.Write(data)
		I = mac.Sum(nil)
		key = append([]byte{}, I[:32]...)
		chainCode = append([]byte{}, I[32:64]...)
	}
	return key
}

// encryptForPublicKeyB64: ephemeralPub(32)|nonce(24)|ciphertext, standard base64.
func encryptForPublicKeyB64(plaintext []byte, recipientPub *[32]byte) string {
	ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return ""
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ""
	}
	sealed := box.Seal(nil, plaintext, &nonce, recipientPub, ephemeralPriv)
	out := make([]byte, 0, 32+24+len(sealed))
	out = append(out, ephemeralPub[:]...)
	out = append(out, nonce[:]...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out)
}

func authResponse(serverURL, bearerToken, pubStd, responseB64 string) error {
	body, _ := json.Marshal(map[string]string{
		"publicKey": pubStd,
		"response":  responseB64,
	})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/v1/auth/response", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
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
