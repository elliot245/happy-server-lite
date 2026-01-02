package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type FeedHandler struct{}

func (h *FeedHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": []any{}, "hasMore": false})
}
