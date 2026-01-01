package hub

import "sync"

type Writer interface {
	Write(message []byte) error
	Close() error
}

type Connection struct {
	UserID string
	Writer Writer
}

type Hub struct {
	mu          sync.RWMutex
	connections map[string]map[*Connection]struct{}
}

func New() *Hub {
	return &Hub{connections: make(map[string]map[*Connection]struct{})}
}

func (h *Hub) Register(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.connections[conn.UserID] == nil {
		h.connections[conn.UserID] = make(map[*Connection]struct{})
	}
	h.connections[conn.UserID][conn] = struct{}{}
}

func (h *Hub) Unregister(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	set := h.connections[conn.UserID]
	if set == nil {
		return
	}
	delete(set, conn)
	if len(set) == 0 {
		delete(h.connections, conn.UserID)
	}
}

func (h *Hub) Broadcast(userID string, message []byte) {
	h.mu.RLock()
	set := h.connections[userID]
	conns := make([]*Connection, 0, len(set))
	for c := range set {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	var failed []*Connection
	for _, c := range conns {
		if err := c.Writer.Write(message); err != nil {
			failed = append(failed, c)
		}
	}
	for _, c := range failed {
		_ = c.Writer.Close()
		h.Unregister(c)
	}
}
