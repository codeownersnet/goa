package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/codeownersnet/goa/session"
)

type inMemoryService struct {
	mu       sync.RWMutex
	memories []Entry
}

func InMemoryService() Service {
	return &inMemoryService{memories: make([]Entry, 0)}
}

func (s *inMemoryService) AddSessionToMemory(_ context.Context, sess session.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for event := range sess.Events().All() {
		var content strings.Builder
		if event.ModelResponse != nil {
			for _, part := range event.ModelResponse.Parts {
				if part.Text != nil {
					content.WriteString(part.Text.Text)
				}
			}
		}
		s.memories = append(s.memories, Entry{
			ID:        event.ID,
			Content:   content.String(),
			Author:    event.Author,
			Timestamp: event.Timestamp,
		})
	}
	return nil
}

func (s *inMemoryService) SearchMemory(_ context.Context, req *SearchRequest) (*SearchResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Entry
	query := strings.ToLower(req.Query)
	for _, mem := range s.memories {
		if strings.Contains(strings.ToLower(mem.Content), query) {
			results = append(results, mem)
		}
	}
	return &SearchResponse{Memories: results}, nil
}

var _ Service = (*inMemoryService)(nil)
