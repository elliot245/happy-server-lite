package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/store"
)

func TestAuthRequestFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	// request
	body, _ := json.Marshal(map[string]any{"publicKey": "pk", "supportsV2": true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// approve
	mobileToken, err := auth.CreateToken("mobile-1", tokenCfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	body, _ = json.Marshal(map[string]any{"publicKey": "pk", "response": "resp"})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/auth/response", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mobileToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// poll again
	body, _ = json.Marshal(map[string]any{"publicKey": "pk", "supportsV2": true})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/auth/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["state"] != "authorized" {
		t.Fatalf("expected state authorized, got %v", resp["state"])
	}
	if resp["token"] == "" || resp["response"] != "resp" {
		t.Fatalf("unexpected auth response: %v", resp)
	}
}

func TestSessionAndMachineEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	userToken, err := auth.CreateToken("user-1", tokenCfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// create session
	body, _ := json.Marshal(map[string]any{"tag": "t1", "metadata": "m1", "agentState": nil})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// create machine
	body, _ = json.Marshal(map[string]any{"id": "m1", "metadata": "mm"})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/machines", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuth_InvalidPublicKeyErrorMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	body, _ := json.Marshal(map[string]any{"publicKey": "not-base64", "challenge": "x", "signature": "y"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid public key") {
		t.Fatalf("expected Invalid public key, got: %s", w.Body.String())
	}
}
