package store

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"happy-server-lite/internal/model"
)

type Store struct {
	mu sync.RWMutex

	accountsByPublicKey map[string]model.Account
	authRequestsByKey   map[string]model.AuthRequest

	sessionsByID       map[string]model.Session
	sessionIDByUserTag map[string]string // userID + "|" + tag -> sessionID

	machinesByID map[string]model.Machine

	messages *messageStore
	seq      *seqGenerator
}

func New() *Store {
	return &Store{
		accountsByPublicKey: make(map[string]model.Account),
		authRequestsByKey:   make(map[string]model.AuthRequest),
		sessionsByID:        make(map[string]model.Session),
		sessionIDByUserTag:  make(map[string]string),
		machinesByID:        make(map[string]model.Machine),
		messages:            newMessageStore(),
		seq:                 newSeqGenerator(),
	}
}

func (s *Store) GetOrCreateAccount(publicKey string, nowMillis int64) (model.Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.accountsByPublicKey[publicKey]; ok {
		return existing, false
	}

	acc := model.Account{
		ID:        uuid.NewString(),
		PublicKey: publicKey,
		CreatedAt: nowMillis,
	}
	s.accountsByPublicKey[publicKey] = acc
	return acc, true
}

func (s *Store) GetAuthRequest(publicKey string) (model.AuthRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	req, ok := s.authRequestsByKey[publicKey]
	return req, ok
}

func (s *Store) UpsertAuthRequest(publicKey string, supportsV2 bool, nowMillis int64) model.AuthRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.authRequestsByKey[publicKey]; ok {
		existing.SupportsV2 = existing.SupportsV2 || supportsV2
		existing.UpdatedAt = nowMillis
		s.authRequestsByKey[publicKey] = existing
		return existing
	}

	req := model.AuthRequest{
		ID:         uuid.NewString(),
		PublicKey:  publicKey,
		SupportsV2: supportsV2,
		CreatedAt:  nowMillis,
		UpdatedAt:  nowMillis,
	}
	s.authRequestsByKey[publicKey] = req
	return req
}

func (s *Store) AuthorizeAuthRequest(publicKey, response, responseAccountID, token string, nowMillis int64) (model.AuthRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.authRequestsByKey[publicKey]
	if !ok {
		return model.AuthRequest{}, false
	}
	req.Response = response
	req.ResponseAccountID = responseAccountID
	req.Token = token
	req.UpdatedAt = nowMillis
	s.authRequestsByKey[publicKey] = req
	return req, true
}

func userTagKey(userID, tag string) string {
	return userID + "|" + tag
}

func (s *Store) GetOrCreateSession(userID, tag, metadata string, agentState *string, dataEncryptionKey *string, nowMillis int64) (model.Session, bool, error) {
	if userID == "" {
		return model.Session{}, false, errors.New("missing userID")
	}
	if tag == "" {
		return model.Session{}, false, errors.New("missing tag")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := userTagKey(userID, tag)
	if sid, ok := s.sessionIDByUserTag[key]; ok {
		sess := s.sessionsByID[sid]
		if sess.Deleted {
			// Treat deleted as new
			delete(s.sessionIDByUserTag, key)
		} else {
			changed := false
			if metadata != "" && metadata != sess.Metadata {
				sess.Metadata = metadata
				sess.MetadataVersion++
				changed = true
			}
			if agentState != nil {
				if sess.AgentState == nil || *sess.AgentState != *agentState {
					sess.AgentState = agentState
					sess.AgentStateVersion++
					changed = true
				}
			}
			if dataEncryptionKey != nil {
				sess.DataEncryptionKey = dataEncryptionKey
				changed = true
			}
			if changed {
				sess.UpdatedAt = nowMillis
				s.sessionsByID[sid] = sess
			}
			return sess, false, nil
		}
	}

	metadataVersion := 0
	if metadata != "" {
		metadataVersion = 1
	}
	agentStateVersion := 0
	if agentState != nil {
		agentStateVersion = 1
	}

	sid := uuid.NewString()
	sess := model.Session{
		ID:                sid,
		UserID:            userID,
		Tag:               tag,
		Seq:               0,
		Metadata:          metadata,
		MetadataVersion:   metadataVersion,
		AgentState:        agentState,
		AgentStateVersion: agentStateVersion,
		DataEncryptionKey: dataEncryptionKey,
		Active:            false,
		ActiveAt:          0,
		CreatedAt:         nowMillis,
		UpdatedAt:         nowMillis,
	}
	s.sessionsByID[sid] = sess
	s.sessionIDByUserTag[key] = sid
	return sess, true, nil
}

func (s *Store) ListSessions(userID string) []model.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Session, 0)
	for _, sess := range s.sessionsByID {
		if sess.UserID == userID && !sess.Deleted {
			result = append(result, sess)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result
}

func (s *Store) GetSession(userID, sessionID string) (model.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessionsByID[sessionID]
	if !ok || sess.UserID != userID || sess.Deleted {
		return model.Session{}, false
	}
	return sess, true
}

func (s *Store) DeleteSession(userID, sessionID string, nowMillis int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessionsByID[sessionID]
	if !ok || sess.UserID != userID || sess.Deleted {
		return false
	}
	sess.Deleted = true
	sess.UpdatedAt = nowMillis
	s.sessionsByID[sessionID] = sess

	// best-effort index cleanup
	key := userTagKey(userID, sess.Tag)
	if s.sessionIDByUserTag[key] == sessionID {
		delete(s.sessionIDByUserTag, key)
	}

	s.messages.deleteSession(sessionID)
	return true
}

func (s *Store) AppendMessage(userID, sessionID, content string, nowMillis int64) (model.SessionMessage, error) {
	_, ok := s.GetSession(userID, sessionID)
	if !ok {
		return model.SessionMessage{}, errors.New("session not found")
	}

	seq := s.seq.nextForSession(sessionID)
	msg := model.SessionMessage{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Seq:       seq,
		Content:   content,
		CreatedAt: nowMillis,
		UpdatedAt: nowMillis,
	}
	s.messages.append(sessionID, msg)
	return msg, nil
}

func (s *Store) ListMessages(userID, sessionID string, after int64, limit int) ([]model.SessionMessage, error) {
	_, ok := s.GetSession(userID, sessionID)
	if !ok {
		return nil, errors.New("session not found")
	}
	if limit <= 0 {
		limit = 100
	}
	return s.messages.getAfter(sessionID, after, limit), nil
}

func (s *Store) UpsertMachine(userID, machineID, metadata string, daemonState *string, dataEncryptionKey *string, nowMillis int64) (model.Machine, bool, error) {
	if machineID == "" {
		return model.Machine{}, false, errors.New("missing machine id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.machinesByID[machineID]; ok {
		if existing.UserID != userID {
			return model.Machine{}, false, errors.New("machine belongs to another user")
		}

		changed := false
		if metadata != "" && metadata != existing.Metadata {
			existing.Metadata = metadata
			existing.MetadataVersion++
			changed = true
		}
		if daemonState != nil {
			if existing.DaemonState == nil || *existing.DaemonState != *daemonState {
				existing.DaemonState = daemonState
				existing.DaemonStateVersion++
				changed = true
			}
		}
		if dataEncryptionKey != nil {
			existing.DataEncryptionKey = dataEncryptionKey
			changed = true
		}
		if changed {
			existing.UpdatedAt = nowMillis
			s.machinesByID[machineID] = existing
		}
		return existing, false, nil
	}

	metadataVersion := 0
	if metadata != "" {
		metadataVersion = 1
	}
	daemonStateVersion := 0
	if daemonState != nil {
		daemonStateVersion = 1
	}

	m := model.Machine{
		ID:                 machineID,
		UserID:             userID,
		Metadata:           metadata,
		MetadataVersion:    metadataVersion,
		DaemonState:        daemonState,
		DaemonStateVersion: daemonStateVersion,
		DataEncryptionKey:  dataEncryptionKey,
		CreatedAt:          nowMillis,
		UpdatedAt:          nowMillis,
	}
	s.machinesByID[machineID] = m
	return m, true, nil
}

func (s *Store) ListMachines(userID string) []model.Machine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Machine, 0)
	for _, m := range s.machinesByID {
		if m.UserID == userID {
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
