package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         int
	MasterSecret string
	GinMode      string
	TLSCertFile  string
	TLSKeyFile   string
	TokenExpiry  time.Duration
}

type Env interface {
	Getenv(key string) string
}

type osEnv struct{}

func (osEnv) Getenv(key string) string { return os.Getenv(key) }

func LoadConfig() (Config, error) {
	return LoadConfigFromEnv(osEnv{})
}

func LoadConfigFromEnv(env Env) (Config, error) {
	cfg := Config{
		Port:        3000,
		GinMode:     "release",
		TokenExpiry: 7 * 24 * time.Hour,
	}

	if raw := env.Getenv("PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port <= 0 || port > 65535 {
			return Config{}, fmt.Errorf("invalid PORT")
		}
		cfg.Port = port
	}

	cfg.MasterSecret = env.Getenv("MASTER_SECRET")
	if cfg.MasterSecret == "" {
		return Config{}, fmt.Errorf("MASTER_SECRET is required")
	}

	if raw := env.Getenv("GIN_MODE"); raw != "" {
		cfg.GinMode = raw
	}

	cfg.TLSCertFile = env.Getenv("TLS_CERT_FILE")
	cfg.TLSKeyFile = env.Getenv("TLS_KEY_FILE")

	if raw := env.Getenv("TOKEN_EXPIRY_SECONDS"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			return Config{}, fmt.Errorf("invalid TOKEN_EXPIRY_SECONDS")
		}
		cfg.TokenExpiry = time.Duration(seconds) * time.Second
	}

	return cfg, nil
}
