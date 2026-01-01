package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/hub"
	"happy-server-lite/internal/store"
)

type WebSocketHandler struct {
	Hub         *hub.Hub
	Store       *store.Store
	TokenConfig auth.TokenConfig
}

type clientMessage struct {
	Type    string `json:"type"`
	SID     string `json:"sid,omitempty"`
	Message string `json:"message,omitempty"`
}

type serverMessage struct {
	Type  string      `json:"type"`
	Event string      `json:"event,omitempty"`
	Body  interface{} `json:"body,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsWriter struct {
	conn *websocket.Conn
}

func (w *wsWriter) Write(message []byte) error {
	w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteMessage(websocket.TextMessage, message)
}

func (w *wsWriter) Close() error {
	return w.conn.Close()
}

func (h *WebSocketHandler) Serve(c *gin.Context) {
	tokenString := c.Query("token")
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}
	claims, err := auth.VerifyToken(tokenString, h.TokenConfig)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	conn := &hub.Connection{UserID: claims.UserID, Writer: &wsWriter{conn: ws}}
	h.Hub.Register(conn)
	defer func() {
		h.Hub.Unregister(conn)
		_ = ws.Close()
	}()

	ws.SetReadLimit(1024 * 1024)
	const pongWait = 60 * time.Second
	const writeWait = 10 * time.Second
	pingPeriod := (pongWait * 9) / 10

	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	done := make(chan struct{})
	var closeOnce sync.Once
	closeDone := func() {
		closeOnce.Do(func() {
			close(done)
		})
	}
	defer closeDone()

	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				deadline := time.Now().Add(writeWait)
				if err := ws.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
					_ = ws.Close()
					return
				}
			}
		}
	}()

	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}

		var msg clientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "ping":
			out, _ := json.Marshal(serverMessage{Type: "pong"})
			_ = conn.Writer.Write(out)
		case "message":
			if msg.SID == "" || msg.Message == "" {
				continue
			}
			now := time.Now().UnixMilli()
			stored, err := h.Store.AppendMessage(claims.UserID, msg.SID, msg.Message, now)
			if err != nil {
				continue
			}
			update := serverMessage{
				Type:  "update",
				Event: "new-message",
				Body: gin.H{
					"t":         "new-message",
					"sessionId": msg.SID,
					"message": gin.H{
						"id":        stored.ID,
						"seq":       stored.Seq,
						"createdAt": stored.CreatedAt,
						"updatedAt": stored.UpdatedAt,
						"content":   gin.H{"t": "encrypted", "c": stored.Content},
					},
				},
			}
			out, _ := json.Marshal(update)
			h.Hub.Broadcast(claims.UserID, out)
		}
	}
}
