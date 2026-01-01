package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
)

var (
	ErrInvalidPublicKey = errors.New("Invalid public key")
	ErrInvalidSignature = errors.New("Invalid signature")
)

func VerifySignature(publicKeyB64, challengeB64, signatureB64 string) bool {
	return VerifySignatureDetailed(publicKeyB64, challengeB64, signatureB64) == nil
}

func VerifySignatureDetailed(publicKeyB64, challengeB64, signatureB64 string) error {
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return ErrInvalidPublicKey
	}

	challenge, err := base64.StdEncoding.DecodeString(challengeB64)
	if err != nil || len(challenge) == 0 {
		return ErrInvalidSignature
	}

	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ErrInvalidSignature
	}

	if !ed25519.Verify(ed25519.PublicKey(publicKey), challenge, signature) {
		return ErrInvalidSignature
	}
	return nil
}
