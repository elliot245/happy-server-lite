package store

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"happy-server-lite/internal/model"
)

type Store struct {
	mu sync.RWMutex

	machinesStateFile string
	persistMu         sync.Mutex

	accountsByPublicKey map[string]model.Account
	authRequestsByKey   map[string]model.AuthRequest

	sessionsByID       map[string]model.Session
	sessionIDByUserTag map[string]string // userID + "|" + tag -> sessionID

	machinesByID   map[string]model.Machine
	artifactsByKey map[string]model.Artifact
	artifactSeq    int64

	accountSettingsByUserID map[string]accountSettings

	messages *messageStore
	seq      *seqGenerator
}

type accountSettings struct {
	Settings *string
	Version  int
}

func New() *Store {
	return NewWithOptions(Options{})
}

type Options struct {
	MachinesStateFile string
}

func NewWithOptions(opts Options) *Store {
	s := &Store{
		accountsByPublicKey:     make(map[string]model.Account),
		authRequestsByKey:       make(map[string]model.AuthRequest),
		sessionsByID:            make(map[string]model.Session),
		sessionIDByUserTag:      make(map[string]string),
		machinesByID:            make(map[string]model.Machine),
		artifactsByKey:          make(map[string]model.Artifact),
		accountSettingsByUserID: make(map[string]accountSettings),
		messages:                newMessageStore(),
		seq:                     newSeqGenerator(),
		machinesStateFile:       opts.MachinesStateFile,
	}

	if s.machinesStateFile != "" {
		if err := s.loadMachinesFromFile(s.machinesStateFile); err != nil {
			log.Printf("machines persistence: load failed (%s): %v", s.machinesStateFile, err)
		}
	}

	return s
}

type persistedMachinesFile struct {
	Version  int             `json:"version"`
	Machines []model.Machine `json:"machines"`
	SavedAt  int64           `json:"savedAt"`
}

func (s *Store) loadMachinesFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var file persistedMachinesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	if file.Version != 1 {
		return errors.New("unsupported machines state version")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range file.Machines {
		if m.ID == "" || m.UserID == "" {
			continue
		}
		s.machinesByID[m.ID] = m
	}
	return nil
}

func (s *Store) snapshotMachinesLocked() []model.Machine {
	result := make([]model.Machine, 0, len(s.machinesByID))
	for _, m := range s.machinesByID {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *Store) persistMachinesSnapshot(machines []model.Machine) {
	path := s.machinesStateFile
	if path == "" {
		return
	}

	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("machines persistence: mkdir failed (%s): %v", dir, err)
		return
	}

	file := persistedMachinesFile{Version: 1, Machines: machines, SavedAt: time.Now().UnixMilli()}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		log.Printf("machines persistence: marshal failed: %v", err)
		return
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		log.Printf("machines persistence: create temp failed: %v", err)
		return
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		log.Printf("machines persistence: chmod temp failed: %v", err)
		return
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		log.Printf("machines persistence: write temp failed: %v", err)
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		log.Printf("machines persistence: sync temp failed: %v", err)
		return
	}
	if err := tmp.Close(); err != nil {
		log.Printf("machines persistence: close temp failed: %v", err)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		log.Printf("machines persistence: rename failed: %v", err)
		return
	}
}

func (s *Store) GetAccountSettings(userID string) (*string, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.accountSettingsByUserID[userID]
	if !ok {
		return nil, 0
	}
	return st.Settings, st.Version
}

func (s *Store) UpdateAccountSettings(userID string, expectedVersion int, settings string, nowMillis int64) (status string, currentVersion int, currentSettings *string) {
	if userID == "" {
		return "error", 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.accountSettingsByUserID[userID]
	if expectedVersion != st.Version {
		return "version-mismatch", st.Version, st.Settings
	}

	st.Version++
	st.Settings = &settings
	s.accountSettingsByUserID[userID] = st
	return "success", st.Version, st.Settings
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

func (s *Store) UpdateSessionMetadata(userID, sessionID string, expectedVersion int, metadata string, nowMillis int64) (status string, version int, currentValue string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessionsByID[sessionID]
	if !ok || sess.UserID != userID || sess.Deleted {
		return "not-found", 0, ""
	}
	if expectedVersion != sess.MetadataVersion {
		return "version-mismatch", sess.MetadataVersion, sess.Metadata
	}

	sess.Metadata = metadata
	sess.MetadataVersion++
	sess.UpdatedAt = nowMillis
	s.sessionsByID[sessionID] = sess
	return "success", sess.MetadataVersion, sess.Metadata
}

func (s *Store) UpdateSessionAgentState(userID, sessionID string, expectedVersion int, agentState *string, nowMillis int64) (status string, version int, currentValue *string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessionsByID[sessionID]
	if !ok || sess.UserID != userID || sess.Deleted {
		return "not-found", 0, nil
	}
	if expectedVersion != sess.AgentStateVersion {
		return "version-mismatch", sess.AgentStateVersion, sess.AgentState
	}

	sess.AgentState = agentState
	sess.AgentStateVersion++
	sess.UpdatedAt = nowMillis
	s.sessionsByID[sessionID] = sess
	return "success", sess.AgentStateVersion, sess.AgentState
}

func (s *Store) SetSessionActive(userID, sessionID string, active bool, activeAt int64, nowMillis int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessionsByID[sessionID]
	if !ok || sess.UserID != userID || sess.Deleted {
		return false
	}
	sess.Active = active
	if active {
		sess.ActiveAt = activeAt
	}
	sess.UpdatedAt = nowMillis
	s.sessionsByID[sessionID] = sess
	return true
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

	if existing, ok := s.machinesByID[machineID]; ok {
		if existing.UserID != userID {
			s.mu.Unlock()
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
		var snapshot []model.Machine
		if changed {
			existing.UpdatedAt = nowMillis
			s.machinesByID[machineID] = existing
			if s.machinesStateFile != "" {
				snapshot = s.snapshotMachinesLocked()
			}
		}
		s.mu.Unlock()
		if snapshot != nil {
			s.persistMachinesSnapshot(snapshot)
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
	var snapshot []model.Machine
	if s.machinesStateFile != "" {
		snapshot = s.snapshotMachinesLocked()
	}
	s.mu.Unlock()
	if snapshot != nil {
		s.persistMachinesSnapshot(snapshot)
	}
	return m, true, nil
}

func (s *Store) GetMachine(userID, machineID string) (model.Machine, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.machinesByID[machineID]
	if !ok || m.UserID != userID {
		return model.Machine{}, false
	}
	return m, true
}

func (s *Store) UpdateMachineMetadata(userID, machineID string, expectedVersion int, metadata string, nowMillis int64) (status string, version int, currentValue string) {
	s.mu.Lock()

	m, ok := s.machinesByID[machineID]
	if !ok || m.UserID != userID {
		s.mu.Unlock()
		return "not-found", 0, ""
	}
	if expectedVersion != m.MetadataVersion {
		s.mu.Unlock()
		return "version-mismatch", m.MetadataVersion, m.Metadata
	}

	m.Metadata = metadata
	m.MetadataVersion++
	m.UpdatedAt = nowMillis
	s.machinesByID[machineID] = m

	var snapshot []model.Machine
	if s.machinesStateFile != "" {
		snapshot = s.snapshotMachinesLocked()
	}
	s.mu.Unlock()
	if snapshot != nil {
		s.persistMachinesSnapshot(snapshot)
	}
	return "success", m.MetadataVersion, m.Metadata
}

func (s *Store) UpdateMachineDaemonState(userID, machineID string, expectedVersion int, daemonState *string, nowMillis int64) (status string, version int, currentValue *string) {
	s.mu.Lock()

	m, ok := s.machinesByID[machineID]
	if !ok || m.UserID != userID {
		s.mu.Unlock()
		return "not-found", 0, nil
	}
	if expectedVersion != m.DaemonStateVersion {
		s.mu.Unlock()
		return "version-mismatch", m.DaemonStateVersion, m.DaemonState
	}

	m.DaemonState = daemonState
	m.DaemonStateVersion++
	m.UpdatedAt = nowMillis
	s.machinesByID[machineID] = m

	var snapshot []model.Machine
	if s.machinesStateFile != "" {
		snapshot = s.snapshotMachinesLocked()
	}
	s.mu.Unlock()
	if snapshot != nil {
		s.persistMachinesSnapshot(snapshot)
	}
	return "success", m.DaemonStateVersion, m.DaemonState
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
