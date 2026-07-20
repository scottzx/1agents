package system

import (
	"encoding/base64"
	"testing"
)

func TestDeriveContentPublicKeyDeterministic(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 1)
	}
	pk1, err := deriveContentPublicKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	pk2, err := deriveContentPublicKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(pk1) != 32 {
		t.Fatalf("pub len %d", len(pk1))
	}
	if base64.StdEncoding.EncodeToString(pk1) != base64.StdEncoding.EncodeToString(pk2) {
		t.Fatal("not deterministic")
	}
	// Different secret → different pub
	secret[0] ^= 0xff
	pk3, err := deriveContentPublicKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	if base64.StdEncoding.EncodeToString(pk1) == base64.StdEncoding.EncodeToString(pk3) {
		t.Fatal("expected different pub for different secret")
	}
}

func TestJwtSub(t *testing.T) {
	// header.payload.sig — payload {"sub":"user123"}
	// echo -n '{"sub":"user123"}' | basenc --base64url -w0
	tok := "eyJhbGciOiJub25lIn0.eyJzdWIiOiJ1c2VyMTIzIn0.x"
	sub, ok := jwtSub(tok)
	if !ok || sub != "user123" {
		t.Fatalf("got %q ok=%v", sub, ok)
	}
}
