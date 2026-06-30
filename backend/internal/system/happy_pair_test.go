package system

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

// buildApprovalBundle mirrors the client's encryptForPublicKey + approveTerminal
// (frontend relay/crypto.ts): box-seal [0x00, contentPublicKey] to the daemon's
// ephemeral public key, laid out as ephemeralPublicKey(32) | nonce(24) | ct.
func buildApprovalBundle(t *testing.T, daemonPub *[32]byte, contentPub []byte) string {
	t.Helper()
	ephPub, ephPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("eph keygen: %v", err)
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	plain := append([]byte{0}, contentPub...) // [0x00, contentPublicKey(32)]
	ct := box.Seal(nil, plain, &nonce, daemonPub, ephPriv)

	bundle := make([]byte, 0, 32+24+len(ct))
	bundle = append(bundle, ephPub[:]...)
	bundle = append(bundle, nonce[:]...)
	bundle = append(bundle, ct...)
	return base64.StdEncoding.EncodeToString(bundle)
}

func TestOpenApprovalResponse_RoundTrip(t *testing.T) {
	daemonPub, daemonPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("daemon keygen: %v", err)
	}
	// Account C's content public key (what the daemon must recover).
	contentPub := make([]byte, 32)
	if _, err := rand.Read(contentPub); err != nil {
		t.Fatalf("contentPub: %v", err)
	}

	respB64 := buildApprovalBundle(t, daemonPub, contentPub)

	got, err := openApprovalResponse(respB64, daemonPriv)
	if err != nil {
		t.Fatalf("openApprovalResponse: %v", err)
	}
	if len(got) != 32 {
		t.Fatalf("recovered key len = %d, want 32", len(got))
	}
	for i := range contentPub {
		if got[i] != contentPub[i] {
			t.Fatalf("recovered content public key mismatch at byte %d", i)
		}
	}
}

func TestOpenApprovalResponse_WrongRecipientFails(t *testing.T) {
	daemonPub, _, _ := box.GenerateKey(rand.Reader)
	_, otherPriv, _ := box.GenerateKey(rand.Reader) // not the daemon's key
	contentPub := make([]byte, 32)
	_, _ = rand.Read(contentPub)

	respB64 := buildApprovalBundle(t, daemonPub, contentPub)

	if _, err := openApprovalResponse(respB64, otherPriv); err == nil {
		t.Fatal("expected decrypt failure with wrong recipient key, got nil")
	}
}

func TestOpenApprovalResponse_ShortBundleFails(t *testing.T) {
	_, priv, _ := box.GenerateKey(rand.Reader)
	if _, err := openApprovalResponse(base64.StdEncoding.EncodeToString([]byte("too short")), priv); err == nil {
		t.Fatal("expected error for short bundle, got nil")
	}
}
