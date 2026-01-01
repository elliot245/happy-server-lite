package config

import "testing"

type mapEnv map[string]string

func (m mapEnv) Getenv(key string) string { return m[key] }

func TestLoadConfigFromEnv_Defaults(t *testing.T) {
	cfg, err := LoadConfigFromEnv(mapEnv{"MASTER_SECRET": "x"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Port != 3000 {
		t.Fatalf("expected default port 3000, got %d", cfg.Port)
	}
	if cfg.GinMode != "release" {
		t.Fatalf("expected default gin mode release, got %q", cfg.GinMode)
	}
}

func TestLoadConfigFromEnv_MissingSecret(t *testing.T) {
	_, err := LoadConfigFromEnv(mapEnv{})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadConfigFromEnv_PortOverride(t *testing.T) {
	cfg, err := LoadConfigFromEnv(mapEnv{"MASTER_SECRET": "x", "PORT": "1234"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Port != 1234 {
		t.Fatalf("expected port 1234, got %d", cfg.Port)
	}
}
