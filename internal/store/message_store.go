package store

import (
	"sync"

	"happy-server-lite/internal/model"
)

type messageStore struct {
	mu   sync.RWMutex
	data map[string][]model.SessionMessage
}

func newMessageStore() *messageStore {
	return &messageStore{data: make(map[string][]model.SessionMessage)}
}

func (m *messageStore) append(sessionID string, msg model.SessionMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[sessionID] = append(m.data[sessionID], msg)
}

func (m *messageStore) getAfter(sessionID string, after int64, limit int) []model.SessionMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msgs := m.data[sessionID]
	if len(msgs) == 0 {
		return nil
	}

	result := make([]model.SessionMessage, 0, limit)
	for _, msg := range msgs {
		if msg.Seq > after {
			result = append(result, msg)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

func (m *messageStore) deleteSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, sessionID)
}
