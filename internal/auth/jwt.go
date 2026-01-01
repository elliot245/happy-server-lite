package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}

type TokenConfig struct {
	Secret string
	Expiry time.Duration
	Issuer string
}

func DefaultTokenConfig(secret string) TokenConfig {
	return TokenConfig{
		Secret: secret,
		Expiry: 7 * 24 * time.Hour,
		Issuer: "happy-server-lite",
	}
}

func CreateToken(userID string, cfg TokenConfig) (string, error) {
	if cfg.Secret == "" {
		return "", errors.New("missing secret")
	}
	if userID == "" {
		return "", errors.New("missing userID")
	}
	if cfg.Expiry <= 0 {
		return "", errors.New("invalid expiry")
	}

	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", err
	}
	jti := hex.EncodeToString(jtiBytes)

	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.Expiry)),
			ID:        jti,
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.Secret))
}

func VerifyToken(tokenString string, cfg TokenConfig) (*Claims, error) {
	if cfg.Secret == "" {
		return nil, errors.New("missing secret")
	}

	parsed, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(cfg.Secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, jwt.ErrSignatureInvalid
	}
	return claims, nil
}
