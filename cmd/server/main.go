package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/config"
	"happy-server-lite/internal/server"
	"happy-server-lite/internal/store"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	gin.SetMode(cfg.GinMode)
	st := store.NewWithOptions(store.Options{MachinesStateFile: cfg.MachinesStateFile})

	tokenCfg := auth.TokenConfig{
		Secret: cfg.MasterSecret,
		Expiry: cfg.TokenExpiry,
		Issuer: "happy-server-lite",
	}

	router := server.NewRouter(server.Deps{Store: st, TokenConfig: tokenCfg})
	log.Printf("listening on %s", fmt.Sprintf(":%d", cfg.Port))
	log.Fatal(server.Run(cfg, router))
}
