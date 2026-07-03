package runner

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/session"
)

type Config struct {
	AppName           string
	Agent             agent.Agent
	SessionService    session.Service
	ArtifactService   artifact.Service
	MemoryService     memory.Service
	AutoCreateSession bool
}

type Runner struct {
	appName           string
	rootAgent         agent.Agent
	sessionService    session.Service
	artifactService   artifact.Service
	memoryService     memory.Service
	autoCreateSession bool
}

func New(cfg Config) (*Runner, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("root agent is required")
	}
	if cfg.SessionService == nil {
		return nil, fmt.Errorf("session service is required")
	}
	return &Runner{
		appName:           cfg.AppName,
		rootAgent:         cfg.Agent,
		sessionService:    cfg.SessionService,
		artifactService:   cfg.ArtifactService,
		memoryService:     cfg.MemoryService,
		autoCreateSession: cfg.AutoCreateSession,
	}, nil
}

func (r *Runner) Run(ctx context.Context, userID, sessionID string, msg *content.Content, cfg agent.RunConfig) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		var storedSession session.Session
		getResp, err := r.sessionService.Get(ctx, &session.GetRequest{
			AppName:   r.appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		if err != nil {
			if !r.autoCreateSession || !errors.Is(err, session.ErrNotFound) {
				yield(nil, fmt.Errorf("get session: %w", err))
				return
			}
			createResp, err := r.sessionService.Create(ctx, &session.CreateRequest{
				AppName:   r.appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			if err != nil {
				yield(nil, err)
				return
			}
			storedSession = createResp.Session
		} else {
			storedSession = getResp.Session
		}

		invCtx := agent.NewInvocationContext(
			ctx,
			r.rootAgent,
			r.artifactService,
			r.memoryService,
			storedSession,
			fmt.Sprintf("inv-%s-%d", sessionID, time.Now().UnixNano()),
			r.rootAgent.Name(),
			nil,
			&cfg,
		)

		if msg != nil {
			userEvent := session.NewEvent(invCtx.InvocationID())
			userEvent.Author = "user"
			userEvent.ModelResponse = msg
			if err := r.sessionService.AppendEvent(ctx, storedSession, userEvent); err != nil {
				yield(nil, fmt.Errorf("failed to append user message: %w", err))
				return
			}
		}

		agentToRun := r.rootAgent
		for event, err := range agentToRun.Run(invCtx) {
			if err != nil {
				if !yield(event, err) {
					return
				}
				continue
			}
			if !event.Partial {
				if err := r.sessionService.AppendEvent(ctx, storedSession, event); err != nil {
					yield(nil, fmt.Errorf("failed to append event: %w", err))
					return
				}
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}
