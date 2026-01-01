package socketio

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"happy-server-lite/internal/auth"
	"happy-server-lite/internal/store"
)

const (
	maxPayload   int64         = 1000000
	writeTimeout time.Duration = 10 * time.Second
)

type Deps struct {
	Store       *store.Store
	TokenConfig auth.TokenConfig
}

type Server struct {
	store       *store.Store
	tokenConfig auth.TokenConfig

	upgrader websocket.Upgrader

	updateSeq int64

	mu            sync.RWMutex
	roomUsers     map[string]map[*conn]struct{}
	roomSessions  map[string]map[*conn]struct{}
	roomMachines  map[string]map[*conn]struct{}
	rpcByMethod   map[string]*conn
	connsBySocket map[*websocket.Conn]*conn
}

func NewServer(deps Deps) *Server {
	return &Server{
		store:       deps.Store,
		tokenConfig: deps.TokenConfig,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		roomUsers:     make(map[string]map[*conn]struct{}),
		roomSessions:  make(map[string]map[*conn]struct{}),
		roomMachines:  make(map[string]map[*conn]struct{}),
		rpcByMethod:   make(map[string]*conn),
		connsBySocket: make(map[*websocket.Conn]*conn),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(maxPayload)

	c := newConn(ws)
	s.registerConn(c)
	defer s.unregisterConn(c)

	open := map[string]any{
		"sid":          c.sid,
		"upgrades":     []string{},
		"pingInterval": 25000,
		"pingTimeout":  20000,
		"maxPayload":   maxPayload,
	}
	openBytes, _ := json.Marshal(open)
	_ = c.writeText(string(engineOpen) + string(openBytes))

	go c.pingLoop()
	c.readLoop(func(msg string) {
		s.handleMessage(c, msg)
	})
}

func (s *Server) registerConn(c *conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connsBySocket[c.ws] = c
}

func (s *Server) unregisterConn(c *conn) {
	s.mu.Lock()
	delete(s.connsBySocket, c.ws)
	if c.userID != "" {
		if c.clientType == "user-scoped" {
			s.leaveRoom(s.roomUsers, c.userID, c)
		}
		if c.sessionID != "" {
			s.leaveRoom(s.roomSessions, c.sessionID, c)
		}
		if c.machineID != "" {
			s.leaveRoom(s.roomMachines, c.machineID, c)
		}
	}
	for method, owner := range s.rpcByMethod {
		if owner == c {
			delete(s.rpcByMethod, method)
		}
	}
	s.mu.Unlock()

	c.close()
}

func (s *Server) joinRoom(rooms map[string]map[*conn]struct{}, key string, c *conn) {
	if key == "" {
		return
	}
	set, ok := rooms[key]
	if !ok {
		set = make(map[*conn]struct{})
		rooms[key] = set
	}
	set[c] = struct{}{}
}

func (s *Server) leaveRoom(rooms map[string]map[*conn]struct{}, key string, c *conn) {
	set, ok := rooms[key]
	if !ok {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(rooms, key)
	}
}

func (s *Server) broadcastToRoom(rooms map[string]map[*conn]struct{}, key string, payload string) {
	if key == "" {
		return
	}

	s.mu.RLock()
	set, ok := rooms[key]
	if !ok {
		s.mu.RUnlock()
		return
	}
	conns := make([]*conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
	}
	s.mu.RUnlock()

	for _, c := range conns {
		if err := c.writeText(string(engineMessage) + payload); err != nil {
			s.unregisterConn(c)
		}
	}
}

func (s *Server) handleMessage(c *conn, msg string) {
	if msg == "" {
		return
	}

	switch enginePacketType(msg[0]) {
	case enginePong:
		c.markPong()
		return
	case engineMessage:
		s.handleSocketPayload(c, msg[1:])
		return
	case engineClose:
		c.close()
		return
	default:
		return
	}
}

type connectAuth struct {
	Token      string `json:"token"`
	ClientType string `json:"clientType"`
	SessionID  string `json:"sessionId"`
	MachineID  string `json:"machineId"`
}

func (s *Server) handleSocketPayload(c *conn, payload string) {
	if payload == "" {
		return
	}

	switch socketPacketType(payload[0]) {
	case socketConnect:
		s.handleConnect(c, payload)
		return
	case socketEvent:
		s.handleEvent(c, payload)
		return
	case socketAck:
		ack, err := parseSocketAckPacket(payload)
		if err != nil {
			return
		}
		c.resolveAck(ack.ID, ack.Args)
		return
	default:
		return
	}
}

func (s *Server) handleConnect(c *conn, payload string) {
	if c.connected.Load() {
		return
	}

	_, rest := parseOptionalNamespace(payload[1:])
	if rest == "" {
		_ = c.writeSocketError("Missing auth")
		c.close()
		return
	}

	var authObj connectAuth
	if err := json.Unmarshal([]byte(rest), &authObj); err != nil {
		_ = c.writeSocketError("Invalid auth")
		c.close()
		return
	}
	if authObj.Token == "" {
		_ = c.writeSocketError("Missing token")
		c.close()
		return
	}
	claims, err := auth.VerifyToken(authObj.Token, s.tokenConfig)
	if err != nil || claims == nil || claims.UserID == "" {
		_ = c.writeSocketError("Invalid authentication token")
		c.close()
		return
	}

	if authObj.ClientType != "user-scoped" && authObj.ClientType != "session-scoped" && authObj.ClientType != "machine-scoped" {
		_ = c.writeSocketError("Invalid client type")
		c.close()
		return
	}

	if authObj.ClientType == "session-scoped" {
		if authObj.SessionID == "" {
			_ = c.writeSocketError("Missing sessionId")
			c.close()
			return
		}
		if _, ok := s.store.GetSession(claims.UserID, authObj.SessionID); !ok {
			_ = c.writeSocketError("Session not found")
			c.close()
			return
		}
	}
	if authObj.ClientType == "machine-scoped" {
		if authObj.MachineID == "" {
			_ = c.writeSocketError("Missing machineId")
			c.close()
			return
		}
		if _, ok := s.store.GetMachine(claims.UserID, authObj.MachineID); !ok {
			_ = c.writeSocketError("Machine not found")
			c.close()
			return
		}
	}

	c.userID = claims.UserID
	c.clientType = authObj.ClientType
	c.sessionID = authObj.SessionID
	c.machineID = authObj.MachineID
	c.connected.Store(true)

	s.mu.Lock()
	if c.clientType == "user-scoped" {
		s.joinRoom(s.roomUsers, c.userID, c)
	}
	if c.sessionID != "" {
		s.joinRoom(s.roomSessions, c.sessionID, c)
	}
	if c.machineID != "" {
		s.joinRoom(s.roomMachines, c.machineID, c)
	}
	s.mu.Unlock()

	_ = c.writeText(string(engineMessage) + string(socketConnect))
}

func (s *Server) handleEvent(c *conn, payload string) {
	if !c.connected.Load() {
		return
	}

	pkt, err := parseSocketEventPacket(payload)
	if err != nil {
		return
	}

	switch pkt.Event {
	case "ping":
		if pkt.ID != nil {
			ackPayload, err := buildSocketAckPacket(pkt.Namespace, *pkt.ID)
			if err == nil {
				_ = c.writeText(string(engineMessage) + ackPayload)
			}
		}
		return

	case "rpc-register":
		var body struct {
			Method string `json:"method"`
		}
		if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.Method == "" {
			return
		}
		s.mu.Lock()
		s.rpcByMethod[body.Method] = c
		s.mu.Unlock()
		return

	case "rpc-unregister":
		var body struct {
			Method string `json:"method"`
		}
		if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.Method == "" {
			return
		}
		s.mu.Lock()
		owner, ok := s.rpcByMethod[body.Method]
		if ok && owner == c {
			delete(s.rpcByMethod, body.Method)
		}
		s.mu.Unlock()
		return

	case "rpc-call":
		if pkt.ID == nil {
			return
		}
		var body struct {
			Method string `json:"method"`
			Params string `json:"params"`
		}
		if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.Method == "" {
			return
		}
		result, err := s.handleRPCCall(body.Method, body.Params)
		resp := gin.H{"ok": err == nil}
		if err != nil {
			resp["error"] = err.Error()
		} else {
			resp["result"] = result
		}
		ackPayload, err2 := buildSocketAckPacket(pkt.Namespace, *pkt.ID, resp)
		if err2 == nil {
			_ = c.writeText(string(engineMessage) + ackPayload)
		}
		return

	case "message":
		s.handleSessionMessage(c, pkt)
		return

	case "update-metadata":
		s.handleSessionMetadataUpdate(c, pkt)
		return

	case "update-state":
		s.handleSessionStateUpdate(c, pkt)
		return

	case "machine-update-metadata":
		s.handleMachineMetadataUpdate(c, pkt)
		return

	case "machine-update-state":
		s.handleMachineStateUpdate(c, pkt)
		return

	case "session-alive":
		var body struct {
			SID  string `json:"sid"`
			Time int64  `json:"time"`
		}
		if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.SID == "" {
			return
		}
		s.store.SetSessionActive(c.userID, body.SID, true, body.Time, time.Now().UnixMilli())
		return

	case "session-end":
		var body struct {
			SID string `json:"sid"`
		}
		if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.SID == "" {
			return
		}
		s.store.SetSessionActive(c.userID, body.SID, false, 0, time.Now().UnixMilli())
		return

	default:
		return
	}
}

func (s *Server) handleRPCCall(method string, params string) (string, error) {
	s.mu.RLock()
	h := s.rpcByMethod[method]
	s.mu.RUnlock()
	if h == nil {
		return "", errors.New("Method not found")
	}

	resp, err := h.emitWithAck("rpc-request", gin.H{"method": method, "params": params}, 10*time.Second)
	if err != nil {
		return "", err
	}
	if len(resp) < 1 {
		return "", errors.New("Empty response")
	}
	var result string
	if err := json.Unmarshal(resp[0], &result); err != nil {
		return "", errors.New("Invalid response")
	}
	return result, nil
}

func (s *Server) nextUpdateID() (string, int64) {
	seq := atomic.AddInt64(&s.updateSeq, 1)
	return uuid.NewString(), seq
}

func (s *Server) handleSessionMessage(c *conn, pkt socketEventPacket) {
	if c.clientType != "session-scoped" {
		return
	}
	var body struct {
		SID     string `json:"sid"`
		Message string `json:"message"`
	}
	if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil {
		return
	}
	if body.SID == "" || body.SID != c.sessionID {
		return
	}

	now := time.Now().UnixMilli()
	msg, err := s.store.AppendMessage(c.userID, body.SID, body.Message, now)
	if err != nil {
		return
	}
	updateID, updateSeq := s.nextUpdateID()
	updatePayload, err := buildSocketEventPacket("/", nil, "update", gin.H{
		"id":        updateID,
		"seq":       updateSeq,
		"createdAt": now,
		"body": gin.H{
			"t":   "new-message",
			"sid": body.SID,
			"message": gin.H{
				"id":  msg.ID,
				"seq": msg.Seq,
				"content": gin.H{
					"t": "encrypted",
					"c": msg.Content,
				},
			},
		},
	})
	if err != nil {
		return
	}

	s.broadcastToRoom(s.roomSessions, body.SID, updatePayload)
	s.broadcastToRoom(s.roomUsers, c.userID, updatePayload)
}

func (s *Server) handleSessionMetadataUpdate(c *conn, pkt socketEventPacket) {
	if pkt.ID == nil {
		return
	}
	var body struct {
		SID             string `json:"sid"`
		ExpectedVersion int    `json:"expectedVersion"`
		Metadata        string `json:"metadata"`
	}
	if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.SID == "" {
		return
	}

	now := time.Now().UnixMilli()
	status, version, value := s.store.UpdateSessionMetadata(c.userID, body.SID, body.ExpectedVersion, body.Metadata, now)
	resp := gin.H{"result": status, "version": version, "metadata": value}
	ackPayload, err := buildSocketAckPacket(pkt.Namespace, *pkt.ID, resp)
	if err == nil {
		_ = c.writeText(string(engineMessage) + ackPayload)
	}
	if status != "success" {
		return
	}

	updateID, updateSeq := s.nextUpdateID()
	updatePayload, err := buildSocketEventPacket("/", nil, "update", gin.H{
		"id":        updateID,
		"seq":       updateSeq,
		"createdAt": now,
		"body": gin.H{
			"t":   "update-session",
			"sid": body.SID,
			"metadata": gin.H{
				"version": version,
				"value":   value,
			},
		},
	})
	if err != nil {
		return
	}
	s.broadcastToRoom(s.roomSessions, body.SID, updatePayload)
	s.broadcastToRoom(s.roomUsers, c.userID, updatePayload)
}

func (s *Server) handleSessionStateUpdate(c *conn, pkt socketEventPacket) {
	if pkt.ID == nil {
		return
	}
	var body struct {
		SID             string  `json:"sid"`
		ExpectedVersion int     `json:"expectedVersion"`
		AgentState      *string `json:"agentState"`
	}
	if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.SID == "" {
		return
	}

	now := time.Now().UnixMilli()
	status, version, value := s.store.UpdateSessionAgentState(c.userID, body.SID, body.ExpectedVersion, body.AgentState, now)
	resp := gin.H{"result": status, "version": version, "agentState": value}
	ackPayload, err := buildSocketAckPacket(pkt.Namespace, *pkt.ID, resp)
	if err == nil {
		_ = c.writeText(string(engineMessage) + ackPayload)
	}
	if status != "success" {
		return
	}

	updateID, updateSeq := s.nextUpdateID()
	updatePayload, err := buildSocketEventPacket("/", nil, "update", gin.H{
		"id":        updateID,
		"seq":       updateSeq,
		"createdAt": now,
		"body": gin.H{
			"t":   "update-session",
			"sid": body.SID,
			"agentState": gin.H{
				"version": version,
				"value":   value,
			},
		},
	})
	if err != nil {
		return
	}
	s.broadcastToRoom(s.roomSessions, body.SID, updatePayload)
	s.broadcastToRoom(s.roomUsers, c.userID, updatePayload)
}

func (s *Server) handleMachineMetadataUpdate(c *conn, pkt socketEventPacket) {
	if pkt.ID == nil {
		return
	}
	var body struct {
		MachineID       string `json:"machineId"`
		ExpectedVersion int    `json:"expectedVersion"`
		Metadata        string `json:"metadata"`
	}
	if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.MachineID == "" {
		return
	}

	now := time.Now().UnixMilli()
	status, version, value := s.store.UpdateMachineMetadata(c.userID, body.MachineID, body.ExpectedVersion, body.Metadata, now)
	resp := gin.H{"result": status, "version": version, "metadata": value}
	ackPayload, err := buildSocketAckPacket(pkt.Namespace, *pkt.ID, resp)
	if err == nil {
		_ = c.writeText(string(engineMessage) + ackPayload)
	}
	if status != "success" {
		return
	}

	updateID, updateSeq := s.nextUpdateID()
	updatePayload, err := buildSocketEventPacket("/", nil, "update", gin.H{
		"id":        updateID,
		"seq":       updateSeq,
		"createdAt": now,
		"body": gin.H{
			"t":         "update-machine",
			"machineId": body.MachineID,
			"metadata": gin.H{
				"version": version,
				"value":   value,
			},
		},
	})
	if err != nil {
		return
	}
	s.broadcastToRoom(s.roomMachines, body.MachineID, updatePayload)
	s.broadcastToRoom(s.roomUsers, c.userID, updatePayload)
}

func (s *Server) handleMachineStateUpdate(c *conn, pkt socketEventPacket) {
	if pkt.ID == nil {
		return
	}
	var body struct {
		MachineID       string  `json:"machineId"`
		ExpectedVersion int     `json:"expectedVersion"`
		DaemonState     *string `json:"daemonState"`
	}
	if len(pkt.Args) < 1 || json.Unmarshal(pkt.Args[0], &body) != nil || body.MachineID == "" {
		return
	}

	now := time.Now().UnixMilli()
	status, version, value := s.store.UpdateMachineDaemonState(c.userID, body.MachineID, body.ExpectedVersion, body.DaemonState, now)
	resp := gin.H{"result": status, "version": version, "daemonState": value}
	ackPayload, err := buildSocketAckPacket(pkt.Namespace, *pkt.ID, resp)
	if err == nil {
		_ = c.writeText(string(engineMessage) + ackPayload)
	}
	if status != "success" {
		return
	}

	updateID, updateSeq := s.nextUpdateID()
	updatePayload, err := buildSocketEventPacket("/", nil, "update", gin.H{
		"id":        updateID,
		"seq":       updateSeq,
		"createdAt": now,
		"body": gin.H{
			"t":         "update-machine",
			"machineId": body.MachineID,
			"daemonState": gin.H{
				"version": version,
				"value":   value,
			},
		},
	})
	if err != nil {
		return
	}
	s.broadcastToRoom(s.roomMachines, body.MachineID, updatePayload)
	s.broadcastToRoom(s.roomUsers, c.userID, updatePayload)
}

type conn struct {
	ws *websocket.Conn

	sid string

	connected atomic.Bool

	userID     string
	clientType string
	sessionID  string
	machineID  string

	sendMu sync.Mutex

	ackMu      sync.Mutex
	nextAckID  int
	pendingAck map[int]chan []json.RawMessage

	pingMu       sync.Mutex
	awaitingPong bool
	pingSentAt   time.Time
	nextPingAt   time.Time

	closed atomic.Bool
}

func newConn(ws *websocket.Conn) *conn {
	return &conn{
		ws:         ws,
		sid:        uuid.NewString(),
		pendingAck: make(map[int]chan []json.RawMessage),
		nextPingAt: time.Now().Add(25 * time.Second),
	}
}

func (c *conn) close() {
	if c.closed.Swap(true) {
		return
	}
	_ = c.ws.Close()
}

func (c *conn) writeText(msg string) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if err := c.ws.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return c.ws.WriteMessage(websocket.TextMessage, []byte(msg))
}

func (c *conn) readLoop(onMessage func(string)) {
	defer c.close()
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		onMessage(string(data))
	}
}

func (c *conn) pingLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if c.closed.Load() {
			return
		}
		now := time.Now()
		c.pingMu.Lock()
		awaiting := c.awaitingPong
		pingSentAt := c.pingSentAt
		nextPingAt := c.nextPingAt
		if awaiting && now.Sub(pingSentAt) > 20*time.Second {
			c.pingMu.Unlock()
			c.close()
			return
		}
		if !awaiting && !now.Before(nextPingAt) {
			c.awaitingPong = true
			c.pingSentAt = now
			c.nextPingAt = now.Add(25 * time.Second)
			c.pingMu.Unlock()
			_ = c.writeText(string(enginePing))
			continue
		}
		c.pingMu.Unlock()
	}
}

func (c *conn) markPong() {
	c.pingMu.Lock()
	c.awaitingPong = false
	c.pingMu.Unlock()
}

func (c *conn) writeSocketError(msg string) error {
	packet, err := buildSocketEventPacket("/", nil, "error", gin.H{"message": msg})
	if err != nil {
		return err
	}
	return c.writeText(string(engineMessage) + packet)
}

func (c *conn) emitWithAck(event string, arg any, timeout time.Duration) ([]json.RawMessage, error) {
	c.ackMu.Lock()
	c.nextAckID++
	id := c.nextAckID
	ch := make(chan []json.RawMessage, 1)
	c.pendingAck[id] = ch
	c.ackMu.Unlock()

	packet, err := buildSocketEventPacket("/", &id, event, arg)
	if err != nil {
		c.ackMu.Lock()
		delete(c.pendingAck, id)
		c.ackMu.Unlock()
		return nil, err
	}
	if err := c.writeText(string(engineMessage) + packet); err != nil {
		c.ackMu.Lock()
		delete(c.pendingAck, id)
		c.ackMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		c.ackMu.Lock()
		delete(c.pendingAck, id)
		c.ackMu.Unlock()
		return nil, errors.New("RPC timeout")
	}
}

func (c *conn) resolveAck(id int, args []json.RawMessage) {
	c.ackMu.Lock()
	ch := c.pendingAck[id]
	delete(c.pendingAck, id)
	c.ackMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- args:
	default:
	}
}
