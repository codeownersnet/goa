package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/session"
)

type Service struct {
	db *sql.DB
}

type Config struct {
	Path string
}

func NewService(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Path == "" {
		cfg.Path = ":memory:"
	}

	db, err := sql.Open("sqlite3", cfg.Path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}

	return &Service{db: db}, nil
}

func (s *Service) Close() error {
	return s.db.Close()
}

func (s *Service) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	if req.AppName == "" || req.UserID == "" {
		return nil, fmt.Errorf("app_name and user_id are required")
	}

	sid := req.SessionID
	if sid == "" {
		sid = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	state := req.State
	if state == nil {
		state = make(map[string]any)
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("sqlite: marshal state: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO sessions (app_name, user_id, session_id, state, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		req.AppName, req.UserID, sid, string(stateJSON), now,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: insert session: %w", err)
	}

	sess := &sqliteSession{
		appName:       req.AppName,
		userID:        req.UserID,
		sessionID:     sid,
		state:         state,
		stateFallback: make(map[string]any),
		updatedAt:     now,
	}

	return &session.CreateResponse{Session: sess}, nil
}

func (s *Service) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id are required")
	}

	var stateJSON string
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT state, updated_at FROM sessions WHERE app_name = ? AND user_id = ? AND session_id = ?`,
		req.AppName, req.UserID, req.SessionID,
	).Scan(&stateJSON, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %s: %w", req.SessionID, session.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: query session: %w", err)
	}

	state := make(map[string]any)
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, fmt.Errorf("sqlite: unmarshal state: %w", err)
	}

	events, err := s.loadEvents(ctx, req.AppName, req.UserID, req.SessionID, req.NumRecentEvents, req.After)
	if err != nil {
		return nil, err
	}

	sess := &sqliteSession{
		appName:       req.AppName,
		userID:        req.UserID,
		sessionID:     req.SessionID,
		state:         state,
		stateFallback: make(map[string]any),
		events:        events,
		updatedAt:     updatedAt,
	}

	return &session.GetResponse{Session: sess}, nil
}

func (s *Service) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	if req.AppName == "" {
		return nil, fmt.Errorf("app_name is required")
	}

	query := `SELECT app_name, user_id, session_id, state, updated_at FROM sessions WHERE app_name = ?`
	args := []any{req.AppName}

	if req.UserID != "" {
		query += ` AND user_id = ?`
		args = append(args, req.UserID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]session.Session, 0)
	for rows.Next() {
		var appName, userID, sessionID, stateJSON string
		var updatedAt time.Time
		if err := rows.Scan(&appName, &userID, &sessionID, &stateJSON, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan session: %w", err)
		}

		state := make(map[string]any)
		if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
			return nil, fmt.Errorf("sqlite: unmarshal listed session state: %w", err)
		}

		sessions = append(sessions, &sqliteSession{
			appName:       appName,
			userID:        userID,
			sessionID:     sessionID,
			state:         state,
			stateFallback: make(map[string]any),
			updatedAt:     updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate sessions: %w", err)
	}

	return &session.ListResponse{Sessions: sessions}, nil
}

func (s *Service) Delete(ctx context.Context, req *session.DeleteRequest) error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`DELETE FROM events WHERE app_name = ? AND user_id = ? AND session_id = ?`,
		req.AppName, req.UserID, req.SessionID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete events: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM sessions WHERE app_name = ? AND user_id = ? AND session_id = ?`,
		req.AppName, req.UserID, req.SessionID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete session: %w", err)
	}

	return tx.Commit()
}

func (s *Service) AppendEvent(ctx context.Context, curSession session.Session, event *session.Event) error {
	if curSession == nil {
		return fmt.Errorf("session is nil")
	}
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.Partial {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	appName := curSession.AppName()
	userID := curSession.UserID()
	sessionID := curSession.ID()

	actionsJSON, err := json.Marshal(event.Actions)
	if err != nil {
		return fmt.Errorf("sqlite: marshal actions: %w", err)
	}

	var modelRespJSON string
	if event.ModelResponse != nil {
		b, err := json.Marshal(event.ModelResponse)
		if err != nil {
			return fmt.Errorf("sqlite: marshal model response: %w", err)
		}
		modelRespJSON = string(b)
	}

	var usageJSON string
	if event.Usage != nil {
		b, err := json.Marshal(event.Usage)
		if err != nil {
			return fmt.Errorf("sqlite: marshal usage: %w", err)
		}
		usageJSON = string(b)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO events (app_name, user_id, session_id, event_id, timestamp, invocation_id, branch, author, partial, actions, model_response, usage, finish_reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		appName, userID, sessionID, event.ID, event.Timestamp, event.InvocationID, event.Branch, event.Author, event.Partial,
		string(actionsJSON), modelRespJSON, usageJSON, string(event.FinishReason),
	)
	if err != nil {
		return fmt.Errorf("sqlite: insert event: %w", err)
	}

	var mergedState map[string]any
	if len(event.Actions.StateDelta) > 0 {
		var stateJSON string
		err = tx.QueryRowContext(ctx,
			`SELECT state FROM sessions WHERE app_name = ? AND user_id = ? AND session_id = ?`,
			appName, userID, sessionID,
		).Scan(&stateJSON)
		if err != nil {
			return fmt.Errorf("sqlite: query state: %w", err)
		}

		mergedState = make(map[string]any)
		if err := json.Unmarshal([]byte(stateJSON), &mergedState); err != nil {
			return fmt.Errorf("sqlite: unmarshal state: %w", err)
		}
		if mergedState == nil {
			mergedState = make(map[string]any)
		}

		for k, v := range event.Actions.StateDelta {
			mergedState[k] = v
		}

		mergedJSON, err := json.Marshal(mergedState)
		if err != nil {
			return fmt.Errorf("sqlite: marshal merged state: %w", err)
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE sessions SET state = ?, updated_at = ? WHERE app_name = ? AND user_id = ? AND session_id = ?`,
			string(mergedJSON), event.Timestamp, appName, userID, sessionID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: update state: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE sessions SET updated_at = ? WHERE app_name = ? AND user_id = ? AND session_id = ?`,
			event.Timestamp, appName, userID, sessionID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: update updated_at: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit: %w", err)
	}

	if sess, ok := curSession.(*sqliteSession); ok {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		if mergedState != nil {
			sess.state = maps.Clone(mergedState)
			sess.stateFallback = maps.Clone(mergedState)
		}
		sess.events = append(sess.events, event)
		sess.updatedAt = event.Timestamp
	}

	return nil
}

func (s *Service) loadEvents(ctx context.Context, appName, userID, sessionID string, numRecent int, after time.Time) ([]*session.Event, error) {
	columns := `event_id, timestamp, invocation_id, branch, author, partial, actions, model_response, usage, finish_reason`
	baseQuery := `SELECT ` + columns + ` FROM events WHERE app_name = ? AND user_id = ? AND session_id = ?`
	limitedBaseQuery := `SELECT rowid AS event_rowid, ` + columns + ` FROM events WHERE app_name = ? AND user_id = ? AND session_id = ?`
	args := []any{appName, userID, sessionID}

	if !after.IsZero() {
		baseQuery += ` AND timestamp > ?`
		limitedBaseQuery += ` AND timestamp > ?`
		args = append(args, after)
	}

	query := baseQuery + ` ORDER BY rowid ASC`
	if numRecent > 0 {
		query = `SELECT ` + columns + ` FROM (` + limitedBaseQuery + ` ORDER BY event_rowid DESC LIMIT ?) ORDER BY event_rowid ASC`
		args = append(args, numRecent)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query events: %w", err)
	}
	defer rows.Close()

	var events []*session.Event
	for rows.Next() {
		var id, invocationID, branch, author string
		var timestamp time.Time
		var partial bool
		var actionsJSON, modelRespJSON, usageJSON sql.NullString
		var finishReason string

		if err := rows.Scan(&id, &timestamp, &invocationID, &branch, &author, &partial, &actionsJSON, &modelRespJSON, &usageJSON, &finishReason); err != nil {
			return nil, fmt.Errorf("sqlite: scan event: %w", err)
		}

		ev := &session.Event{
			ID:           id,
			Timestamp:    timestamp,
			InvocationID: invocationID,
			Branch:       branch,
			Author:       author,
			Partial:      partial,
			FinishReason: model.FinishReason(finishReason),
		}

		if actionsJSON.Valid && actionsJSON.String != "" {
			if err := json.Unmarshal([]byte(actionsJSON.String), &ev.Actions); err != nil {
				return nil, fmt.Errorf("sqlite: unmarshal actions: %w", err)
			}
		}

		if modelRespJSON.Valid && modelRespJSON.String != "" {
			var mr content.Content
			if err := json.Unmarshal([]byte(modelRespJSON.String), &mr); err != nil {
				return nil, fmt.Errorf("sqlite: unmarshal model response: %w", err)
			}
			ev.ModelResponse = &mr
		}

		if usageJSON.Valid && usageJSON.String != "" {
			var u model.Usage
			if err := json.Unmarshal([]byte(usageJSON.String), &u); err != nil {
				return nil, fmt.Errorf("sqlite: unmarshal usage: %w", err)
			}
			ev.Usage = &u
		}

		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate events: %w", err)
	}

	return events, nil
}

type sqliteSession struct {
	appName       string
	userID        string
	sessionID     string
	mu            sync.RWMutex
	state         map[string]any
	stateFallback map[string]any
	events        []*session.Event
	updatedAt     time.Time
}

func (s *sqliteSession) ID() string                { return s.sessionID }
func (s *sqliteSession) AppName() string           { return s.appName }
func (s *sqliteSession) UserID() string            { return s.userID }
func (s *sqliteSession) LastUpdateTime() time.Time { return s.updatedAt }

func (s *sqliteSession) State() session.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		s.state = s.stateFallback
	}
	return &sqliteState{mu: &s.mu, state: s.state}
}

func (s *sqliteSession) Events() session.Events {
	return sqliteEvents(s.events)
}

var _ session.Session = (*sqliteSession)(nil)

type sqliteState struct {
	mu    *sync.RWMutex
	state map[string]any
}

func (s *sqliteState) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.state[key]
	if !ok {
		return nil, session.ErrStateKeyNotExist
	}
	return val, nil
}

func (s *sqliteState) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state[key] = value
	return nil
}

func (s *sqliteState) All() iter.Seq2[string, any] {
	s.mu.RLock()
	snapshot := maps.Clone(s.state)
	s.mu.RUnlock()

	return func(yield func(string, any) bool) {
		for k, v := range snapshot {
			if !yield(k, v) {
				return
			}
		}
	}
}

var _ session.State = (*sqliteState)(nil)

type sqliteEvents []*session.Event

func (e sqliteEvents) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, event := range e {
			if !yield(event) {
				return
			}
		}
	}
}

func (e sqliteEvents) Len() int { return len(e) }
func (e sqliteEvents) At(i int) *session.Event {
	if i >= 0 && i < len(e) {
		return e[i]
	}
	return nil
}

var _ session.Events = sqliteEvents(nil)

func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			app_name   TEXT NOT NULL,
			user_id    TEXT NOT NULL,
			session_id TEXT NOT NULL,
			state      TEXT NOT NULL DEFAULT '{}',
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (app_name, user_id, session_id)
		);
		CREATE TABLE IF NOT EXISTS events (
			app_name    TEXT NOT NULL,
			user_id     TEXT NOT NULL,
			session_id  TEXT NOT NULL,
			event_id    TEXT NOT NULL,
			timestamp   DATETIME NOT NULL,
			invocation_id TEXT NOT NULL DEFAULT '',
			branch      TEXT NOT NULL DEFAULT '',
			author       TEXT NOT NULL DEFAULT '',
			partial      BOOLEAN NOT NULL DEFAULT 0,
			actions      TEXT NOT NULL DEFAULT '{}',
			model_response TEXT NOT NULL DEFAULT '',
			usage        TEXT NOT NULL DEFAULT '',
			finish_reason TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_events_session
			ON events (app_name, user_id, session_id);
	`)
	if err != nil {
		return fmt.Errorf("sqlite: migrate exec: %w", err)
	}
	return nil
}
