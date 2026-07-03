package session

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"
)

type stateMap map[string]any

type inMemoryService struct {
	mu       sync.RWMutex
	sessions map[string]*session
}

type session struct {
	id        sessionKey
	mu        sync.RWMutex
	events    []*Event
	state     stateMap
	updatedAt time.Time
}

type sessionKey struct {
	appName   string
	userID    string
	sessionID string
}

func (s sessionKey) encode() string {
	return s.appName + "/" + s.userID + "/" + s.sessionID
}

func InMemoryService() Service {
	return &inMemoryService{
		sessions: make(map[string]*session),
	}
}

func (s *inMemoryService) Create(_ context.Context, req *CreateRequest) (*CreateResponse, error) {
	if req.AppName == "" || req.UserID == "" {
		return nil, fmt.Errorf("app_name and user_id are required")
	}
	sid := req.SessionID
	if sid == "" {
		sid = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	key := sessionKey{appName: req.AppName, userID: req.UserID, sessionID: sid}.encode()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[key]; ok {
		return nil, fmt.Errorf("session %s already exists", req.SessionID)
	}

	state := req.State
	if state == nil {
		state = make(stateMap)
	}
	val := &session{
		id:        sessionKey{appName: req.AppName, userID: req.UserID, sessionID: sid},
		state:     state,
		updatedAt: time.Now(),
	}
	s.sessions[key] = val

	return &CreateResponse{Session: copySession(val)}, nil
}

func (s *inMemoryService) Get(_ context.Context, req *GetRequest) (*GetResponse, error) {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id are required")
	}
	key := sessionKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.encode()

	s.mu.RLock()
	defer s.mu.RUnlock()

	res, ok := s.sessions[key]
	if !ok {
		return nil, fmt.Errorf("session %s: %w", req.SessionID, ErrNotFound)
	}

	copied := copySession(res)
	filteredEvents := res.events
	if req.NumRecentEvents > 0 {
		start := max(len(filteredEvents)-req.NumRecentEvents, 0)
		filteredEvents = filteredEvents[start:]
	}
	if !req.After.IsZero() && len(filteredEvents) > 0 {
		firstIndex := sort.Search(len(filteredEvents), func(i int) bool {
			return !filteredEvents[i].Timestamp.Before(req.After)
		})
		filteredEvents = filteredEvents[firstIndex:]
	}
	copied.events = make([]*Event, len(filteredEvents))
	copy(copied.events, filteredEvents)

	return &GetResponse{Session: copied}, nil
}

func (s *inMemoryService) List(_ context.Context, req *ListRequest) (*ListResponse, error) {
	if req.AppName == "" {
		return nil, fmt.Errorf("app_name is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sessions []Session
	for _, stored := range s.sessions {
		if stored.id.appName == req.AppName {
			if req.UserID == "" || stored.id.userID == req.UserID {
				sessions = append(sessions, copySession(stored))
			}
		}
	}
	return &ListResponse{Sessions: sessions}, nil
}

func (s *inMemoryService) Delete(_ context.Context, req *DeleteRequest) error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required")
	}
	key := sessionKey{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.encode()

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
	return nil
}

func (s *inMemoryService) AppendEvent(_ context.Context, curSession Session, event *Event) error {
	if curSession == nil {
		return fmt.Errorf("session is nil")
	}
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.Partial {
		return nil
	}

	sess, ok := curSession.(*session)
	if !ok {
		return fmt.Errorf("unexpected session type %T", curSession)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.sessions[sess.id.encode()]
	if !ok {
		return fmt.Errorf("session %s: %w", sess.id.sessionID, ErrNotFound)
	}

	if len(event.Actions.StateDelta) > 0 {
		maps.Copy(stored.state, event.Actions.StateDelta)
	}
	stored.events = append(stored.events, event)
	stored.updatedAt = event.Timestamp

	if sess != stored {
		if len(event.Actions.StateDelta) > 0 {
			maps.Copy(sess.state, event.Actions.StateDelta)
		}
		sess.events = append(sess.events, event)
		sess.updatedAt = event.Timestamp
	}
	return nil
}

func copySession(s *session) *session {
	return &session{
		id:        s.id,
		state:     maps.Clone(s.state),
		events:    slices.Clone(s.events),
		updatedAt: s.updatedAt,
	}
}

func (s *session) ID() string {
	return s.id.sessionID
}

func (s *session) AppName() string {
	return s.id.appName
}

func (s *session) UserID() string {
	return s.id.userID
}

func (s *session) State() State {
	return &state{mu: &s.mu, state: s.state}
}

func (s *session) Events() Events {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return events(s.events)
}

func (s *session) LastUpdateTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

type events []*Event

func (e events) All() iter.Seq[*Event] {
	return func(yield func(*Event) bool) {
		for _, event := range e {
			if !yield(event) {
				return
			}
		}
	}
}

func (e events) Len() int {
	return len(e)
}

func (e events) At(i int) *Event {
	if i >= 0 && i < len(e) {
		return e[i]
	}
	return nil
}

type state struct {
	mu    *sync.RWMutex
	state map[string]any
}

func (s *state) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.state[key]
	if !ok {
		return nil, ErrStateKeyNotExist
	}
	return val, nil
}

func (s *state) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[key] = value
	return nil
}

func (s *state) All() iter.Seq2[string, any] {
	s.mu.RLock()
	stateCopy := maps.Clone(s.state)
	s.mu.RUnlock()
	return func(yield func(key string, val any) bool) {
		for k, v := range stateCopy {
			if !yield(k, v) {
				return
			}
		}
	}
}

var _ Service = (*inMemoryService)(nil)
