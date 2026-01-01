package server

import (
	"fmt"
	"net/http"
	"time"

	"happy-server-lite/internal/config"
)

func NewHTTPServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func Run(cfg config.Config, handler http.Handler) error {
	srv := NewHTTPServer(cfg, handler)
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		return srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	}
	return srv.ListenAndServe()
}
