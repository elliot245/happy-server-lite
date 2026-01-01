package server

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/store"
)

func waitForPrefix(t *testing.T, c *websocket.Conn, prefix string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, data, err := c.ReadMessage()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			t.Fatalf("ReadMessage: %v", err)
		}
		msg := string(data)
		if msg == "2" {
			_ = c.WriteMessage(websocket.TextMessage, []byte("3"))
			continue
		}
		if strings.HasPrefix(msg, prefix) {
			_ = c.SetReadDeadline(time.Time{})
			return msg
		}
	}
	t.Fatalf("timeout waiting for %q", prefix)
	return ""
}

func TestSocketIOHandshakeAndPingAck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	userToken, err := auth.CreateToken("user-1", tokenCfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	sess, _, err := st.GetOrCreateSession("user-1", "tag", "m", nil, nil, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/updates/?EIO=4&transport=websocket"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	open := waitForPrefix(t, conn, "0{", 2*time.Second)
	if !strings.Contains(open, "\"pingInterval\"") {
		t.Fatalf("unexpected open packet: %s", open)
	}

	authPayload := map[string]any{"token": userToken, "clientType": "session-scoped", "sessionId": sess.ID}
	authBytes, _ := json.Marshal(authPayload)
	if err := conn.WriteMessage(websocket.TextMessage, []byte("40"+string(authBytes))); err != nil {
		t.Fatalf("WriteMessage(connect): %v", err)
	}
	_ = waitForPrefix(t, conn, "40", 2*time.Second)

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`421["ping"]`)); err != nil {
		t.Fatalf("WriteMessage(ping): %v", err)
	}
	ack := waitForPrefix(t, conn, "431", 2*time.Second)
	if ack != "431[]" {
		t.Fatalf("unexpected ack: %s", ack)
	}
}

func TestSocketIOUpdateBroadcastToUserScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	userToken, err := auth.CreateToken("user-1", tokenCfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	sess, _, err := st.GetOrCreateSession("user-1", "tag", "m", nil, nil, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	srv := httptest.NewServer(r)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/updates/?EIO=4&transport=websocket"

	userConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial(user): %v", err)
	}
	defer userConn.Close()
	_ = waitForPrefix(t, userConn, "0{", 2*time.Second)
	userAuth := map[string]any{"token": userToken, "clientType": "user-scoped"}
	userAuthBytes, _ := json.Marshal(userAuth)
	if err := userConn.WriteMessage(websocket.TextMessage, []byte("40"+string(userAuthBytes))); err != nil {
		t.Fatalf("WriteMessage(user connect): %v", err)
	}
	_ = waitForPrefix(t, userConn, "40", 2*time.Second)

	sessConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial(session): %v", err)
	}
	defer sessConn.Close()
	_ = waitForPrefix(t, sessConn, "0{", 2*time.Second)
	sessAuth := map[string]any{"token": userToken, "clientType": "session-scoped", "sessionId": sess.ID}
	sessAuthBytes, _ := json.Marshal(sessAuth)
	if err := sessConn.WriteMessage(websocket.TextMessage, []byte("40"+string(sessAuthBytes))); err != nil {
		t.Fatalf("WriteMessage(session connect): %v", err)
	}
	_ = waitForPrefix(t, sessConn, "40", 2*time.Second)

	msgPayload := map[string]any{"sid": sess.ID, "message": "enc"}
	msgBytes, _ := json.Marshal(msgPayload)
	if err := sessConn.WriteMessage(websocket.TextMessage, []byte(`42["message",`+string(msgBytes)+`]`)); err != nil {
		t.Fatalf("WriteMessage(message): %v", err)
	}

	updateRaw := waitForPrefix(t, userConn, "42", 2*time.Second)
	var arr []any
	if err := json.Unmarshal([]byte(updateRaw[2:]), &arr); err != nil {
		t.Fatalf("unmarshal update: %v (%s)", err, updateRaw)
	}
	if len(arr) < 2 || arr[0] != "update" {
		t.Fatalf("unexpected update event: %v", arr)
	}
	body, ok := arr[1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected update body: %T", arr[1])
	}
	bodyObj, _ := body["body"].(map[string]any)
	if bodyObj["t"] != "new-message" {
		t.Fatalf("unexpected update type: %v", bodyObj["t"])
	}
}
