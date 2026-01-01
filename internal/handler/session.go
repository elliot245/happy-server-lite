package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/middleware"
	"happy-server-lite/internal/store"
)

type SessionHandler struct {
	Store *store.Store
}

type createSessionBody struct {
	Tag               string  `json:"tag"`
	Metadata          string  `json:"metadata"`
	AgentState        *string `json:"agentState"`
	DataEncryptionKey *string `json:"dataEncryptionKey"`
}

func (h *SessionHandler) GetOrCreate(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	var body createSessionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	now := time.Now().UnixMilli()
	sess, _, err := h.Store.GetOrCreateSession(userID, body.Tag, body.Metadata, body.AgentState, body.DataEncryptionKey, now)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"session": gin.H{
		"id":                sess.ID,
		"tag":               sess.Tag,
		"seq":               sess.Seq,
		"createdAt":         sess.CreatedAt,
		"updatedAt":         sess.UpdatedAt,
		"metadata":          sess.Metadata,
		"metadataVersion":   sess.MetadataVersion,
		"agentState":        sess.AgentState,
		"agentStateVersion": sess.AgentStateVersion,
		"dataEncryptionKey": sess.DataEncryptionKey,
		"active":            sess.Active,
		"activeAt":          sess.ActiveAt,
		"lastMessage":       nil,
	}})
}

func (h *SessionHandler) List(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	sessions := h.Store.ListSessions(userID)
	resp := make([]gin.H, 0, len(sessions))
	for _, sess := range sessions {
		resp = append(resp, gin.H{
			"id":                sess.ID,
			"tag":               sess.Tag,
			"seq":               sess.Seq,
			"createdAt":         sess.CreatedAt,
			"updatedAt":         sess.UpdatedAt,
			"metadata":          sess.Metadata,
			"metadataVersion":   sess.MetadataVersion,
			"agentState":        sess.AgentState,
			"agentStateVersion": sess.AgentStateVersion,
			"dataEncryptionKey": sess.DataEncryptionKey,
			"active":            sess.Active,
			"activeAt":          sess.ActiveAt,
			"lastMessage":       nil,
		})
	}
	c.JSON(http.StatusOK, gin.H{"sessions": resp})
}

func (h *SessionHandler) Delete(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session id"})
		return
	}

	if !h.Store.DeleteSession(userID, sessionID, time.Now().UnixMilli()) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *SessionHandler) Messages(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session id"})
		return
	}

	after := int64(0)
	if raw := c.Query("after"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor format"})
			return
		}
		after = v
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor format"})
			return
		}
		limit = v
	}

	msgs, err := h.Store.ListMessages(userID, sessionID, after, limit)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	resp := make([]gin.H, 0, len(msgs))
	for _, m := range msgs {
		resp = append(resp, gin.H{
			"id":        m.ID,
			"seq":       m.Seq,
			"createdAt": m.CreatedAt,
			"updatedAt": m.UpdatedAt,
			"content": gin.H{
				"t": "encrypted",
				"c": m.Content,
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{"messages": resp})
}
