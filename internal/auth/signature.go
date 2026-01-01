package auth

import (
	"crypto/ed25519"
	"encoding/base64"
)

func VerifySignature(publicKeyB64, challengeB64, signatureB64 string) bool {
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return false
	}

	challenge, err := base64.StdEncoding.DecodeString(challengeB64)
	if err != nil || len(challenge) == 0 {
		return false
	}

	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(ed25519.PublicKey(publicKey), challenge, signature)
}
