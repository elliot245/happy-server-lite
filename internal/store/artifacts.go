package store

import (
	"errors"
	"sort"

	"happy-server-lite/internal/model"
)

type ArtifactUpdateResult struct {
	Success bool

	HeaderVersion *int
	BodyVersion   *int

	CurrentHeaderVersion *int
	CurrentBodyVersion   *int
	CurrentHeader        *string
	CurrentBody          *string
}

func artifactKey(userID, artifactID string) string {
	return userID + "|" + artifactID
}

func (s *Store) ListArtifacts(userID string) []model.Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Artifact, 0)
	for _, a := range s.artifactsByKey {
		if a.UserID == userID && !a.Deleted {
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt == result[j].UpdatedAt {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt > result[j].UpdatedAt
	})
	return result
}

func (s *Store) GetArtifact(userID, artifactID string) (model.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.artifactsByKey[artifactKey(userID, artifactID)]
	if !ok || a.UserID != userID || a.Deleted {
		return model.Artifact{}, false
	}
	return a, true
}

func (s *Store) CreateArtifact(userID, artifactID, header, body, dataEncryptionKey string, nowMillis int64) (model.Artifact, bool, error) {
	if userID == "" {
		return model.Artifact{}, false, errors.New("missing user id")
	}
	if artifactID == "" {
		return model.Artifact{}, false, errors.New("missing artifact id")
	}
	if header == "" || body == "" || dataEncryptionKey == "" {
		return model.Artifact{}, false, errors.New("missing artifact fields")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := artifactKey(userID, artifactID)
	if existing, ok := s.artifactsByKey[key]; ok && !existing.Deleted {
		return existing, false, nil
	}

	s.artifactSeq++
	a := model.Artifact{
		ID:               artifactID,
		UserID:           userID,
		Header:           header,
		HeaderVersion:    1,
		Body:             body,
		BodyVersion:      1,
		DataEncryptionKey: dataEncryptionKey,
		Seq:              s.artifactSeq,
		CreatedAt:        nowMillis,
		UpdatedAt:        nowMillis,
	}
	s.artifactsByKey[key] = a
	return a, true, nil
}

func (s *Store) UpdateArtifact(userID, artifactID string, header *string, expectedHeaderVersion *int, body *string, expectedBodyVersion *int, nowMillis int64) (ArtifactUpdateResult, error) {
	if userID == "" {
		return ArtifactUpdateResult{}, errors.New("missing user id")
	}
	if artifactID == "" {
		return ArtifactUpdateResult{}, errors.New("missing artifact id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := artifactKey(userID, artifactID)
	a, ok := s.artifactsByKey[key]
	if !ok || a.UserID != userID || a.Deleted {
		return ArtifactUpdateResult{}, errors.New("artifact not found")
	}

	if header != nil {
		if expectedHeaderVersion == nil || *expectedHeaderVersion != a.HeaderVersion {
			chv := a.HeaderVersion
			cbv := a.BodyVersion
			ch := a.Header
			cb := a.Body
			return ArtifactUpdateResult{
				Success:             false,
				CurrentHeaderVersion: &chv,
				CurrentBodyVersion:   &cbv,
				CurrentHeader:        &ch,
				CurrentBody:          &cb,
			}, nil
		}
		a.Header = *header
		a.HeaderVersion++
	}

	if body != nil {
		if expectedBodyVersion == nil || *expectedBodyVersion != a.BodyVersion {
			chv := a.HeaderVersion
			cbv := a.BodyVersion
			ch := a.Header
			cb := a.Body
			return ArtifactUpdateResult{
				Success:             false,
				CurrentHeaderVersion: &chv,
				CurrentBodyVersion:   &cbv,
				CurrentHeader:        &ch,
				CurrentBody:          &cb,
			}, nil
		}
		a.Body = *body
		a.BodyVersion++
	}

	// No-op updates still succeed
	a.UpdatedAt = nowMillis
	s.artifactSeq++
	a.Seq = s.artifactSeq
	s.artifactsByKey[key] = a

	res := ArtifactUpdateResult{Success: true}
	if header != nil {
		hv := a.HeaderVersion
		res.HeaderVersion = &hv
	}
	if body != nil {
		bv := a.BodyVersion
		res.BodyVersion = &bv
	}
	return res, nil
}

func (s *Store) DeleteArtifact(userID, artifactID string) bool {
	if userID == "" || artifactID == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := artifactKey(userID, artifactID)
	a, ok := s.artifactsByKey[key]
	if !ok || a.UserID != userID || a.Deleted {
		return false
	}
	a.Deleted = true
	s.artifactsByKey[key] = a
	return true
}
