package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/store"
)

func TestWebSocketPingPong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	tok, err := auth.CreateToken("user-1", tokenCfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token=" + tok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"type": "ping"}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var resp map[string]any
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	data, _ := json.Marshal(resp)
	if resp["type"] != "pong" {
		t.Fatalf("expected pong, got %s", string(data))
	}
}
