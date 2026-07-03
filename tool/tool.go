package tool

import (
	"context"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/session"
)

type Tool interface {
	Name() string
	Description() string
	Process(ctx context.Context, args map[string]any) (map[string]any, error)
}

type Declarer interface {
	Declaration() *content.ToolDeclaration
}

type Toolset interface {
	Tools() []Tool
}

type ResourceInfo struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type ResourceProvider interface {
	Resources() []ResourceInfo
	ReadResource(ctx context.Context, uri string) ([]byte, error)
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type PromptInfo struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptProvider interface {
	Prompts() []PromptInfo
	GetPrompt(ctx context.Context, name string, args map[string]string) (string, error)
}

type Context interface {
	agent.CallbackContext
	FunctionCallID() string
	Actions() *EventActions
}

type EventActions struct {
	StateDelta        map[string]any
	TransferToAgent   string
	Escalate          bool
	SkipSummarization bool
}

type actionsKey struct{}

func ContextWithActions(ctx context.Context, actions *EventActions) context.Context {
	return context.WithValue(ctx, actionsKey{}, actions)
}

func ActionsFromContext(ctx context.Context) *EventActions {
	if actions, ok := ctx.Value(actionsKey{}).(*EventActions); ok {
		return actions
	}
	return nil
}

type stateKey struct{}

func ContextWithState(ctx context.Context, state session.State) context.Context {
	return context.WithValue(ctx, stateKey{}, state)
}

func StateFromContext(ctx context.Context) session.State {
	if state, ok := ctx.Value(stateKey{}).(session.State); ok {
		return state
	}
	return nil
}
