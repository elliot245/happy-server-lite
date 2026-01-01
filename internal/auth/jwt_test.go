package auth

import (
	"testing"
	"time"
)

func TestCreateAndVerifyToken(t *testing.T) {
	cfg := TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	tok, err := CreateToken("user-1", cfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	claims, err := VerifyToken(tok, cfg)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("expected user-1, got %q", claims.UserID)
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	cfg := TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	tok, err := CreateToken("user-1", cfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	_, err = VerifyToken(tok, TokenConfig{Secret: "wrong", Expiry: time.Hour, Issuer: "test"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	cfg := TokenConfig{Secret: "secret", Expiry: -time.Second, Issuer: "test"}
	_, err := CreateToken("user-1", cfg)
	if err == nil {
		t.Fatalf("expected error")
	}
}
