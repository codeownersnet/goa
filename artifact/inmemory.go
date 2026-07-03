package artifact

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

type inMemoryService struct {
	mu        sync.RWMutex
	artifacts map[string]*storedArtifact
}

type artifactKey struct {
	appName   string
	userID    string
	sessionID string
	fileName  string
	version   int64
}

func (k artifactKey) encode() string {
	return fmt.Sprintf("%s/%s/%s/%s/%d", k.appName, k.userID, k.sessionID, k.fileName, k.version)
}

type storedArtifact struct {
	data     []byte
	mimeType string
	version  int64
}

func InMemoryService() Service {
	return &inMemoryService{artifacts: make(map[string]*storedArtifact)}
}

func (s *inMemoryService) Save(_ context.Context, req *SaveRequest) (*SaveResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	nextVersion := int64(1)
	prefix := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName}.encode()
	for key, artifact := range s.artifacts {
		if strings.HasPrefix(key, prefix) {
			if artifact.version >= nextVersion {
				nextVersion = artifact.version + 1
			}
		}
	}

	key := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName, version: nextVersion}.encode()
	s.artifacts[key] = &storedArtifact{data: req.Data, mimeType: req.MIMEType, version: nextVersion}
	return &SaveResponse{Version: nextVersion}, nil
}

func (s *inMemoryService) Load(_ context.Context, req *LoadRequest) (*LoadResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if req.Version > 0 {
		key := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName, version: req.Version}.encode()
		art, ok := s.artifacts[key]
		if !ok {
			return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
		}
		return &LoadResponse{Data: art.data, MIMEType: art.mimeType}, nil
	}

	var latest *storedArtifact
	prefix := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName}.encode()
	for key, art := range s.artifacts {
		if strings.HasPrefix(key, prefix) {
			if latest == nil || art.version > latest.version {
				latest = art
			}
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}
	return &LoadResponse{Data: latest.data, MIMEType: latest.mimeType}, nil
}

func (s *inMemoryService) Delete(_ context.Context, req *DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.Version > 0 {
		key := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName, version: req.Version}.encode()
		delete(s.artifacts, key)
		return nil
	}

	prefix := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName}.encode()
	for key := range s.artifacts {
		if strings.HasPrefix(key, prefix) {
			delete(s.artifacts, key)
		}
	}
	return nil
}

func (s *inMemoryService) List(_ context.Context, req *ListRequest) (*ListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	files := map[string]bool{}
	prefix := req.AppName + "/" + req.UserID + "/" + req.SessionID + "/"
	for key := range s.artifacts {
		if strings.HasPrefix(key, prefix) {
			parts := strings.Split(key, "/")
			if len(parts) >= 4 {
				files[parts[3]] = true
			}
		}
	}
	var filenames []string
	for name := range files {
		filenames = append(filenames, name)
	}
	sort.Strings(filenames)
	return &ListResponse{FileNames: filenames}, nil
}

func (s *inMemoryService) Versions(_ context.Context, req *VersionsRequest) (*VersionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var versions []int64
	prefix := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName}.encode()
	for key, art := range s.artifacts {
		if strings.HasPrefix(key, prefix) {
			versions = append(versions, art.version)
		}
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] > versions[j] })
	return &VersionsResponse{Versions: versions}, nil
}

func (s *inMemoryService) GetArtifactVersion(_ context.Context, req *GetArtifactVersionRequest) (*GetArtifactVersionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var art *storedArtifact
	if req.Version > 0 {
		key := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName, version: req.Version}.encode()
		art = s.artifacts[key]
	} else {
		prefix := artifactKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID, fileName: req.FileName}.encode()
		for key, a := range s.artifacts {
			if strings.HasPrefix(key, prefix) {
				if art == nil || a.version > art.version {
					art = a
				}
			}
		}
	}

	if art == nil {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	return &GetArtifactVersionResponse{
		ArtifactVersion: &ArtifactVersion{
			Version:  art.version,
			MIMEType: art.mimeType,
		},
	}, nil
}

var _ Service = (*inMemoryService)(nil)
