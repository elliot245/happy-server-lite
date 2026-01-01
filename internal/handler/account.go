package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/middleware"
	"happy-server-lite/internal/store"
)

type AccountHandler struct {
	Store *store.Store
}

func (h *AccountHandler) Profile(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                userID,
		"timestamp":         time.Now().UnixMilli(),
		"firstName":         nil,
		"lastName":          nil,
		"avatar":            nil,
		"github":            nil,
		"connectedServices": []string{},
	})
}

func (h *AccountHandler) Settings(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	settings, version := h.Store.GetAccountSettings(userID)
	c.JSON(http.StatusOK, gin.H{"settings": settings, "settingsVersion": version})
}

type updateSettingsBody struct {
	Settings        string `json:"settings"`
	ExpectedVersion int    `json:"expectedVersion"`
}

func (h *AccountHandler) UpdateSettings(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	var body updateSettingsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if body.Settings == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing settings"})
		return
	}

	status, currentVersion, currentSettings := h.Store.UpdateAccountSettings(userID, body.ExpectedVersion, body.Settings, time.Now().UnixMilli())
	if status == "success" {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	if status == "version-mismatch" {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "version-mismatch", "currentVersion": currentVersion, "currentSettings": currentSettings})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "error"})
}
