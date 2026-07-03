package memory

import (
	"context"
	"time"

	"github.com/codeownersnet/goa/session"
)

type Service interface {
	AddSessionToMemory(ctx context.Context, s session.Session) error
	SearchMemory(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
}

type SearchRequest struct {
	Query   string
	UserID  string
	AppName string
}

type SearchResponse struct {
	Memories []Entry
}

type Entry struct {
	ID             string
	Content        string
	Author         string
	Timestamp      time.Time
	CustomMetadata map[string]any
}
