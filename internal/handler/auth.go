package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/middleware"
	"happy-server-lite/internal/store"
)

type AuthHandler struct {
	Store              *store.Store
	TokenConfig        auth.TokenConfig
	AuthRequestLimiter *middleware.RateLimiter
}

type authRequestBody struct {
	PublicKey  string `json:"publicKey"`
	SupportsV2 bool   `json:"supportsV2"`
}

type authResponseBody struct {
	PublicKey string `json:"publicKey"`
	Response  string `json:"response"`
}

type authBody struct {
	PublicKey string `json:"publicKey"`
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
}

func (h *AuthHandler) Auth(c *gin.Context) {
	var body authBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := auth.VerifySignatureDetailed(body.PublicKey, body.Challenge, body.Signature); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UnixMilli()
	account, _ := h.Store.GetOrCreateAccount(body.PublicKey, now)
	token, err := auth.CreateToken(account.ID, h.TokenConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "token": token})
}

func (h *AuthHandler) Request(c *gin.Context) {
	var body authRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if body.PublicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid public key"})
		return
	}

	// Polling should not be rate-limited; only creation is.
	if _, ok := h.Store.GetAuthRequest(body.PublicKey); !ok {
		if h.AuthRequestLimiter != nil && !h.AuthRequestLimiter.Allow(c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
			return
		}
	}

	now := time.Now().UnixMilli()
	req := h.Store.UpsertAuthRequest(body.PublicKey, body.SupportsV2, now)

	if req.Token != "" {
		c.JSON(http.StatusOK, gin.H{
			"state":      "authorized",
			"token":      req.Token,
			"response":   req.Response,
			"supportsV2": req.SupportsV2,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"state":      "requested",
		"supportsV2": req.SupportsV2,
	})
}

func (h *AuthHandler) Response(c *gin.Context) {
	var body authResponseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if body.PublicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid public key"})
		return
	}
	if body.Response == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid response"})
		return
	}

	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	now := time.Now().UnixMilli()
	account, _ := h.Store.GetOrCreateAccount(body.PublicKey, now)
	token, err := auth.CreateToken(account.ID, h.TokenConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token creation failed"})
		return
	}

	_, authorized := h.Store.AuthorizeAuthRequest(body.PublicKey, body.Response, userID, token, now)
	if !authorized {
		c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *AuthHandler) RequestStatus(c *gin.Context) {
	publicKey := c.Query("publicKey")
	if publicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid public key"})
		return
	}

	req, ok := h.Store.GetAuthRequest(publicKey)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "not_found"})
		return
	}
	if req.Token == "" {
		c.JSON(http.StatusOK, gin.H{"status": "pending", "supportsV2": req.SupportsV2})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "authorized", "supportsV2": req.SupportsV2})
}
