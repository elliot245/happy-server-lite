package server

import (
	"net/http"
	"testing"
	"time"

	"happy-server-lite/internal/config"
)

func TestNewHTTPServer(t *testing.T) {
	cfg := config.Config{Port: 4321, MasterSecret: "x"}
	srv := NewHTTPServer(cfg, http.NewServeMux())
	if srv.Addr != ":4321" {
		t.Fatalf("expected :4321, got %q", srv.Addr)
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("unexpected ReadHeaderTimeout")
	}
}
