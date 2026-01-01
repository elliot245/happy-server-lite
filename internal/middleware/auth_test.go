package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/auth"
)

func TestRequireAuth_SetsUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "secret"
	tok, err := auth.CreateToken("user-1", auth.TokenConfig{Secret: secret, Expiry: time.Hour, Issuer: "test"})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	r := gin.New()
	r.GET("/", RequireAuth(auth.TokenConfig{Secret: secret, Expiry: time.Hour, Issuer: "test"}), func(c *gin.Context) {
		uid, ok := UserIDFromContext(c)
		if !ok || uid != "user-1" {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
