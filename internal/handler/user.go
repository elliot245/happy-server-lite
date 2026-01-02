package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserHandler struct{}

func (h *UserHandler) Search(c *gin.Context) {
	// Keep response schema stable for mobile clients.
	c.JSON(http.StatusOK, gin.H{"users": []any{}})
}

func (h *UserHandler) Get(c *gin.Context) {
	// Not implemented: return 404 so clients can treat as missing.
	_ = c.Param("id")
	c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
}
