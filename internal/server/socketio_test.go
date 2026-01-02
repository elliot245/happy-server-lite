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

func TestSocketIOMachineAliveBroadcastsEphemeral(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	tokenCfg := auth.TokenConfig{Secret: "secret", Expiry: time.Hour, Issuer: "test"}
	r := NewRouter(Deps{Store: st, TokenConfig: tokenCfg})

	userToken, err := auth.CreateToken("user-1", tokenCfg)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	_, _, err = st.UpsertMachine("user-1", "m1", "mm", nil, nil, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("UpsertMachine: %v", err)
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

	machineConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial(machine): %v", err)
	}
	defer machineConn.Close()
	_ = waitForPrefix(t, machineConn, "0{", 2*time.Second)
	machineAuth := map[string]any{"token": userToken, "clientType": "machine-scoped", "machineId": "m1"}
	machineAuthBytes, _ := json.Marshal(machineAuth)
	if err := machineConn.WriteMessage(websocket.TextMessage, []byte("40"+string(machineAuthBytes))); err != nil {
		t.Fatalf("WriteMessage(machine connect): %v", err)
	}
	_ = waitForPrefix(t, machineConn, "40", 2*time.Second)

	alivePayload := map[string]any{"machineId": "m1", "time": float64(123)}
	aliveBytes, _ := json.Marshal(alivePayload)
	if err := machineConn.WriteMessage(websocket.TextMessage, []byte(`42["machine-alive",`+string(aliveBytes)+`]`)); err != nil {
		t.Fatalf("WriteMessage(machine-alive): %v", err)
	}

	ephemeralRaw := waitForPrefix(t, userConn, "42", 2*time.Second)
	var arr []any
	if err := json.Unmarshal([]byte(ephemeralRaw[2:]), &arr); err != nil {
		t.Fatalf("unmarshal ephemeral: %v (%s)", err, ephemeralRaw)
	}
	if len(arr) < 2 || arr[0] != "ephemeral" {
		t.Fatalf("unexpected event: %v", arr)
	}
	data, ok := arr[1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected ephemeral body: %T", arr[1])
	}
	if data["type"] != "machine-activity" || data["id"] != "m1" {
		t.Fatalf("unexpected ephemeral body: %v", data)
	}
	if data["active"] != true {
		t.Fatalf("unexpected active: %v", data["active"])
	}
}

func TestSocketIOHandshakeOnUserMachineDaemonPath(t *testing.T) {
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

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/user-machine-daemon/?EIO=4&transport=websocket"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	_ = waitForPrefix(t, conn, "0{", 2*time.Second)

	authPayload := map[string]any{"token": userToken, "clientType": "session-scoped", "sessionId": sess.ID}
	authBytes, _ := json.Marshal(authPayload)
	if err := conn.WriteMessage(websocket.TextMessage, []byte("40"+string(authBytes))); err != nil {
		t.Fatalf("WriteMessage(connect): %v", err)
	}
	_ = waitForPrefix(t, conn, "40", 2*time.Second)
}

func TestSocketIOSendMessageFromUserScopedBroadcastToSessionScoped(t *testing.T) {
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

	msgPayload := map[string]any{"sid": sess.ID, "message": "enc", "localId": "local-1"}
	msgBytes, _ := json.Marshal(msgPayload)
	if err := userConn.WriteMessage(websocket.TextMessage, []byte(`42["message",`+string(msgBytes)+`]`)); err != nil {
		t.Fatalf("WriteMessage(message): %v", err)
	}

	updateRaw := waitForPrefix(t, sessConn, "42", 2*time.Second)
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
	msg, _ := bodyObj["message"].(map[string]any)
	if msg["localId"] != "local-1" {
		t.Fatalf("expected localId local-1, got: %v", msg["localId"])
	}
	if createdAt, ok := msg["createdAt"].(float64); !ok || createdAt <= 0 {
		t.Fatalf("unexpected createdAt: %v", msg["createdAt"])
	}
}
