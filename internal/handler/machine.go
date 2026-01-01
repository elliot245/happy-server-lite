package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/middleware"
	"happy-server-lite/internal/store"
)

type MachineHandler struct {
	Store *store.Store
}

type upsertMachineBody struct {
	ID                string  `json:"id"`
	Tag               string  `json:"tag"`
	Metadata          string  `json:"metadata"`
	DaemonState       *string `json:"daemonState"`
	DataEncryptionKey *string `json:"dataEncryptionKey"`
}

func (h *MachineHandler) Upsert(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	var body upsertMachineBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	machineID := body.ID
	if machineID == "" {
		machineID = body.Tag
	}

	now := time.Now().UnixMilli()
	m, _, err := h.Store.UpsertMachine(userID, machineID, body.Metadata, body.DaemonState, body.DataEncryptionKey, now)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"machine": gin.H{
		"id":                 m.ID,
		"createdAt":          m.CreatedAt,
		"updatedAt":          m.UpdatedAt,
		"metadata":           m.Metadata,
		"metadataVersion":    m.MetadataVersion,
		"daemonState":        m.DaemonState,
		"daemonStateVersion": m.DaemonStateVersion,
	}})
}

func (h *MachineHandler) List(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	machines := h.Store.ListMachines(userID)
	resp := make([]gin.H, 0, len(machines))
	for _, m := range machines {
		resp = append(resp, gin.H{
			"id":                 m.ID,
			"createdAt":          m.CreatedAt,
			"updatedAt":          m.UpdatedAt,
			"metadata":           m.Metadata,
			"metadataVersion":    m.MetadataVersion,
			"daemonState":        m.DaemonState,
			"daemonStateVersion": m.DaemonStateVersion,
		})
	}
	c.JSON(http.StatusOK, gin.H{"machines": resp})
}
