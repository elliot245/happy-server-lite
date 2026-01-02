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
	issuedToken, _ := resp["token"].(string)
	claims, err := auth.VerifyToken(issuedToken, tokenCfg)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if claims.UserID != "mobile-1" {
		t.Fatalf("expected issued token for mobile-1, got %q", claims.UserID)
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

	// list machines (iOS expects a top-level JSON array)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/machines", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var machines []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &machines); err != nil {
		t.Fatalf("unmarshal machines: %v (%s)", err, w.Body.String())
	}
	if len(machines) != 1 {
		t.Fatalf("expected 1 machine, got %d: %v", len(machines), machines)
	}
	m := machines[0]
	if m["id"] != "m1" {
		t.Fatalf("unexpected machine id: %v", m["id"])
	}
	if m["metadata"] != "mm" {
		t.Fatalf("unexpected metadata: %v", m["metadata"])
	}
	if m["metadataVersion"] != float64(1) {
		t.Fatalf("unexpected metadataVersion: %v", m["metadataVersion"])
	}
	if _, ok := m["daemonState"]; !ok {
		t.Fatalf("expected daemonState key")
	}
	if m["daemonStateVersion"] != float64(0) {
		t.Fatalf("unexpected daemonStateVersion: %v", m["daemonStateVersion"])
	}
	if m["seq"] != float64(0) {
		t.Fatalf("unexpected seq: %v", m["seq"])
	}
	if m["active"] != false {
		t.Fatalf("unexpected active: %v", m["active"])
	}
	if m["activeAt"] != float64(0) {
		t.Fatalf("unexpected activeAt: %v", m["activeAt"])
	}
	if _, ok := m["dataEncryptionKey"]; !ok {
		t.Fatalf("expected dataEncryptionKey key")
	}
	if createdAt, ok := m["createdAt"].(float64); !ok || createdAt <= 0 {
		t.Fatalf("unexpected createdAt: %v", m["createdAt"])
	}
	if updatedAt, ok := m["updatedAt"].(float64); !ok || updatedAt <= 0 {
		t.Fatalf("unexpected updatedAt: %v", m["updatedAt"])
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

func TestWelcomeAndVersionEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Welcome to Happy Server!") {
		t.Fatalf("expected welcome body, got: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/version", bytes.NewReader([]byte(`{"platform":"ios","version":"1.0","app_id":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "update_required") {
		t.Fatalf("expected update_required, got: %s", w.Body.String())
	}
}

func TestAccountSettingsVersionMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	userToken, err := auth.CreateToken("user-1", tokenCfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// initial GET should return settings null and version 0
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/account/settings", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// update with expectedVersion 0 should succeed
	body, _ := json.Marshal(map[string]any{"settings": "enc", "expectedVersion": 0})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/account/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// update with expectedVersion 0 again should version-mismatch
	body, _ = json.Marshal(map[string]any{"settings": "enc2", "expectedVersion": 0})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/account/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "version-mismatch") {
		t.Fatalf("expected version-mismatch, got: %s", w.Body.String())
	}
}
