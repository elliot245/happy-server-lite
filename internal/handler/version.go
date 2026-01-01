package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type VersionHandler struct{}

func (h *VersionHandler) Check(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"update_required": false})
}
