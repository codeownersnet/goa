package session

import (
	"context"
	"time"
)

type Creator interface {
	Create(context.Context, *CreateRequest) (*CreateResponse, error)
}

type Getter interface {
	Get(context.Context, *GetRequest) (*GetResponse, error)
}

type Lister interface {
	List(context.Context, *ListRequest) (*ListResponse, error)
}

type Deleter interface {
	Delete(context.Context, *DeleteRequest) error
}

type EventAppender interface {
	AppendEvent(context.Context, Session, *Event) error
}

type Service interface {
	Creator
	Getter
	Lister
	Deleter
	EventAppender
}

type CreateRequest struct {
	AppName   string
	UserID    string
	SessionID string
	State     map[string]any
}

type CreateResponse struct {
	Session Session
}

type GetRequest struct {
	AppName         string
	UserID          string
	SessionID       string
	NumRecentEvents int
	After           time.Time
}

type GetResponse struct {
	Session Session
}

type ListRequest struct {
	AppName string
	UserID  string
}

type ListResponse struct {
	Sessions []Session
}

type DeleteRequest struct {
	AppName   string
	UserID    string
	SessionID string
}
