package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type FriendsHandler struct{}

func (h *FriendsHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"friends": []any{}})
}

func (h *FriendsHandler) Add(c *gin.Context) {
	// Minimal compatibility: avoid 404s; full social graph is out of scope for happy-server-lite.
	var body struct {
		UID string `json:"uid"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.UID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": dummyUserProfile(body.UID, "requested")})
}

func (h *FriendsHandler) Remove(c *gin.Context) {
	var body struct {
		UID string `json:"uid"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.UID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": dummyUserProfile(body.UID, "none")})
}

func dummyUserProfile(id string, status string) gin.H {
	return gin.H{
		"id":        id,
		"firstName": "User",
		"lastName":  nil,
		"avatar":    nil,
		"username":  id,
		"bio":       nil,
		"status":    status,
	}
}
