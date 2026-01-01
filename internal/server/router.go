package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/handler"
	"happy-server-lite/internal/hub"
	"happy-server-lite/internal/middleware"
	"happy-server-lite/internal/socketio"
	"happy-server-lite/internal/store"
)

type Deps struct {
	Store       *store.Store
	TokenConfig auth.TokenConfig
}

func NewRouter(deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Welcome to Happy Server!")
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	authRequestLimiter := middleware.NewRateLimiter(10, time.Minute)
	authHandler := &handler.AuthHandler{Store: deps.Store, TokenConfig: deps.TokenConfig, AuthRequestLimiter: authRequestLimiter}

	r.POST("/v1/auth", authHandler.Auth)
	r.POST("/v1/auth/request", authHandler.Request)
	r.POST("/v1/auth/account/request", authHandler.Request)
	r.GET("/v1/auth/request/status", authHandler.RequestStatus)

	versionHandler := &handler.VersionHandler{}
	r.POST("/v1/version", versionHandler.Check)

	protected := r.Group("/v1")
	protected.Use(middleware.RequireAuth(deps.TokenConfig))
	protected.POST("/auth/response", authHandler.Response)
	protected.POST("/auth/account/response", authHandler.Response)

	accountHandler := &handler.AccountHandler{Store: deps.Store}
	protected.GET("/account/profile", accountHandler.Profile)
	protected.GET("/account/settings", accountHandler.Settings)
	protected.POST("/account/settings", accountHandler.UpdateSettings)

	sessionHandler := &handler.SessionHandler{Store: deps.Store}
	protected.GET("/sessions", sessionHandler.List)
	protected.POST("/sessions", sessionHandler.GetOrCreate)
	protected.DELETE("/sessions/:id", sessionHandler.Delete)
	protected.GET("/sessions/:id/messages", sessionHandler.Messages)

	machineHandler := &handler.MachineHandler{Store: deps.Store}
	protected.GET("/machines", machineHandler.List)
	protected.POST("/machines", machineHandler.Upsert)

	wsHub := hub.New()
	wsHandler := &handler.WebSocketHandler{Hub: wsHub, Store: deps.Store, TokenConfig: deps.TokenConfig}
	r.GET("/ws", wsHandler.Serve)

	sio := socketio.NewServer(socketio.Deps{Store: deps.Store, TokenConfig: deps.TokenConfig})
	r.Any("/v1/updates", gin.WrapH(sio))
	r.Any("/v1/updates/*any", gin.WrapH(sio))

	return r
}
