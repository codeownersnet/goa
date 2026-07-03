package session

import (
	"errors"
	"iter"
	"time"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
)

type Session interface {
	ID() string
	AppName() string
	UserID() string
	Events() Events
	State() State
	LastUpdateTime() time.Time
}

type State interface {
	Get(string) (any, error)
	Set(string, any) error
	All() iter.Seq2[string, any]
}

type ReadonlyState interface {
	Get(string) (any, error)
	All() iter.Seq2[string, any]
}

type Events interface {
	All() iter.Seq[*Event]
	Len() int
	At(i int) *Event
}

type Event struct {
	ID            string
	Timestamp     time.Time
	InvocationID  string
	Branch        string
	Author        string
	Actions       EventActions
	Partial       bool
	ModelResponse *content.Content
	Usage         *model.Usage
	FinishReason  model.FinishReason
}

type EventActions struct {
	StateDelta        map[string]any
	ArtifactDelta     map[string]int64
	TransferToAgent   string
	Escalate          bool
	SkipSummarization bool
}

var ErrNotFound = errors.New("session not found")

var ErrStateKeyNotExist = errors.New("state key does not exist")
