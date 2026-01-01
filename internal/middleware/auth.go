package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/auth"
)

const userIDContextKey = "userID"

func UserIDFromContext(c *gin.Context) (string, bool) {
	userID, ok := c.Get(userIDContextKey)
	if !ok {
		return "", false
	}
	value, ok := userID.(string)
	return value, ok && value != ""
}

func RequireAuth(cfg auth.TokenConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			c.Abort()
			return
		}

		claims, err := auth.VerifyToken(parts[1], cfg)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			c.Abort()
			return
		}

		c.Set(userIDContextKey, claims.UserID)
		c.Next()
	}
}
