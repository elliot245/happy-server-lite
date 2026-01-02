package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/middleware"
	"happy-server-lite/internal/store"
)

type ArtifactHandler struct {
	Store *store.Store
}

type createArtifactBody struct {
	ID               string `json:"id"`
	Header           string `json:"header"`
	Body             string `json:"body"`
	DataEncryptionKey string `json:"dataEncryptionKey"`
}

type updateArtifactBody struct {
	Header               *string `json:"header"`
	ExpectedHeaderVersion *int    `json:"expectedHeaderVersion"`
	Body                 *string `json:"body"`
	ExpectedBodyVersion   *int    `json:"expectedBodyVersion"`
}

func (h *ArtifactHandler) List(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	artifacts := h.Store.ListArtifacts(userID)
	resp := make([]gin.H, 0, len(artifacts))
	for _, a := range artifacts {
		resp = append(resp, gin.H{
			"id":               a.ID,
			"header":           a.Header,
			"headerVersion":    a.HeaderVersion,
			"dataEncryptionKey": a.DataEncryptionKey,
			"seq":              a.Seq,
			"createdAt":        a.CreatedAt,
			"updatedAt":        a.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, resp)
}

func (h *ArtifactHandler) Get(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	artifactID := c.Param("id")
	if artifactID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid artifact id"})
		return
	}

	a, ok := h.Store.GetArtifact(userID, artifactID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":               a.ID,
		"header":           a.Header,
		"headerVersion":    a.HeaderVersion,
		"body":             a.Body,
		"bodyVersion":      a.BodyVersion,
		"dataEncryptionKey": a.DataEncryptionKey,
		"seq":              a.Seq,
		"createdAt":        a.CreatedAt,
		"updatedAt":        a.UpdatedAt,
	})
}

func (h *ArtifactHandler) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	var body createArtifactBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	now := time.Now().UnixMilli()
	a, created, err := h.Store.CreateArtifact(userID, body.ID, body.Header, body.Body, body.DataEncryptionKey, now)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{"error": "Artifact already exists"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":               a.ID,
		"header":           a.Header,
		"headerVersion":    a.HeaderVersion,
		"body":             a.Body,
		"bodyVersion":      a.BodyVersion,
		"dataEncryptionKey": a.DataEncryptionKey,
		"seq":              a.Seq,
		"createdAt":        a.CreatedAt,
		"updatedAt":        a.UpdatedAt,
	})
}

func (h *ArtifactHandler) Update(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	artifactID := c.Param("id")
	if artifactID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid artifact id"})
		return
	}

	var body updateArtifactBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	now := time.Now().UnixMilli()
	res, err := h.Store.UpdateArtifact(userID, artifactID, body.Header, body.ExpectedHeaderVersion, body.Body, body.ExpectedBodyVersion, now)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact not found"})
		return
	}
	if res.Success {
		resp := gin.H{"success": true}
		if res.HeaderVersion != nil {
			resp["headerVersion"] = *res.HeaderVersion
		}
		if res.BodyVersion != nil {
			resp["bodyVersion"] = *res.BodyVersion
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	resp := gin.H{"success": false, "error": "version-mismatch"}
	if res.CurrentHeaderVersion != nil {
		resp["currentHeaderVersion"] = *res.CurrentHeaderVersion
	}
	if res.CurrentBodyVersion != nil {
		resp["currentBodyVersion"] = *res.CurrentBodyVersion
	}
	if res.CurrentHeader != nil {
		resp["currentHeader"] = *res.CurrentHeader
	}
	if res.CurrentBody != nil {
		resp["currentBody"] = *res.CurrentBody
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ArtifactHandler) Delete(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	artifactID := c.Param("id")
	if artifactID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid artifact id"})
		return
	}

	if !h.Store.DeleteArtifact(userID, artifactID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact not found"})
		return
	}
	// client only checks response.ok
	c.JSON(http.StatusOK, gin.H{"success": true})
}
