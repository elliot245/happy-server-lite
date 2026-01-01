package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestVerifySignature_Valid(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	sig := ed25519.Sign(priv, challenge)

	if !VerifySignature(
		base64.StdEncoding.EncodeToString(pub),
		base64.StdEncoding.EncodeToString(challenge),
		base64.StdEncoding.EncodeToString(sig),
	) {
		t.Fatalf("expected signature to verify")
	}
}

func TestVerifySignature_InvalidLengths(t *testing.T) {
	if VerifySignature("", "", "") {
		t.Fatalf("expected false")
	}

	// public key wrong length
	if VerifySignature(base64.StdEncoding.EncodeToString([]byte{1, 2, 3}), base64.StdEncoding.EncodeToString([]byte{1}), base64.StdEncoding.EncodeToString(make([]byte, 64))) {
		t.Fatalf("expected false")
	}
}

func TestVerifySignature_InvalidBase64(t *testing.T) {
	if VerifySignature("not-base64", "not-base64", "not-base64") {
		t.Fatalf("expected false")
	}
}
