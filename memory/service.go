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
	ID             string         `json:"id"`
	AppName        string         `json:"app_name"`
	UserID         string         `json:"user_id"`
	Content        string         `json:"content"`
	Author         string         `json:"author"`
	Timestamp      time.Time      `json:"timestamp"`
	CustomMetadata map[string]any `json:"custom_metadata,omitempty"`
}
