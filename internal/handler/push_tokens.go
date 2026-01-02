package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type PushTokensHandler struct{}

func (h *PushTokensHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"tokens": []any{}})
}

func (h *PushTokensHandler) Register(c *gin.Context) {
	// Minimal compatibility: accept and acknowledge; persistence not required for happy-server-lite.
	var body struct {
		Token string `json:"token"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
