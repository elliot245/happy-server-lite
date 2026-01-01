package store

import "sync"

type seqGenerator struct {
	mu         sync.Mutex
	perSession map[string]int64
}

func newSeqGenerator() *seqGenerator {
	return &seqGenerator{perSession: make(map[string]int64)}
}

func (g *seqGenerator) nextForSession(sessionID string) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.perSession[sessionID]++
	return g.perSession[sessionID]
}
