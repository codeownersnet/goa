package agent

import (
	"context"

	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/session"
)

type InvocationContext interface {
	context.Context
	Agent() Agent
	ArtifactService() artifact.Service
	MemoryService() memory.Service
	Session() session.Session
	InvocationID() string
	Branch() string
	UserContent() *content.Content
	RunConfig() *RunConfig
	EndInvocation()
	Ended() bool
	WithContext(ctx context.Context) InvocationContext
}

type ReadonlyContext interface {
	context.Context
	UserContent() *content.Content
	InvocationID() string
	AgentName() string
	State() session.State
	UserID() string
	AppName() string
	SessionID() string
	Branch() string
}

type CallbackContext interface {
	ReadonlyContext
	Artifacts() artifact.Service
	State() session.State
}
