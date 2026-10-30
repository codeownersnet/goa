package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/codeownersnet/goa/session"
)

type Option func(*serviceOptions)

type serviceOptions struct {
	maxEntries int
}

func WithMaxEntries(n int) Option {
	return func(o *serviceOptions) {
		o.maxEntries = n
	}
}

type inMemoryService struct {
	mu         sync.RWMutex
	memories   []Entry
	maxEntries int
}

func InMemoryService(opts ...Option) Service {
	var options serviceOptions
	for _, opt := range opts {
		opt(&options)
	}
	return &inMemoryService{
		memories:   make([]Entry, 0),
		maxEntries: options.maxEntries,
	}
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
			AppName:   sess.AppName(),
			UserID:    sess.UserID(),
			Content:   content.String(),
			Author:    event.Author,
			Timestamp: event.Timestamp,
		})
	}
	if s.maxEntries > 0 && len(s.memories) > s.maxEntries {
		s.memories = s.memories[len(s.memories)-s.maxEntries:]
	}
	return nil
}

func (s *inMemoryService) SearchMemory(_ context.Context, req *SearchRequest) (*SearchResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Entry
	query := strings.ToLower(req.Query)
	for _, mem := range s.memories {
		if req.AppName != "" && mem.AppName != req.AppName {
			continue
		}
		if req.UserID != "" && mem.UserID != req.UserID {
			continue
		}
		if strings.Contains(strings.ToLower(mem.Content), query) {
			results = append(results, mem)
		}
	}
	return &SearchResponse{Memories: results}, nil
}

var _ Service = (*inMemoryService)(nil)
