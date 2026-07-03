package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type agentTool struct {
	wrappedAgent      agent.Agent
	skipSummarization bool
	impl              tool.Tool
}

type Config struct {
	SkipSummarization bool
}

func New(a agent.Agent, cfg *Config) (tool.Tool, error) {
	skip := false
	if cfg != nil {
		skip = cfg.SkipSummarization
	}
	at := &agentTool{
		wrappedAgent:      a,
		skipSummarization: skip,
	}

	ft, err := functiontool.New(functiontool.Config{
		Name:        at.Name(),
		Description: at.Description(),
	}, at.process)
	if err != nil {
		return nil, fmt.Errorf("agenttool %s: create tool: %w", a.Name(), err)
	}
	at.impl = ft
	return at, nil
}

func (t *agentTool) Name() string {
	return t.wrappedAgent.Name()
}

func (t *agentTool) Description() string {
	return t.wrappedAgent.Description()
}

func (t *agentTool) Process(ctx context.Context, args map[string]any) (map[string]any, error) {
	return t.process(ctx, args)
}

func (t *agentTool) process(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.skipSummarization {
		if actions := tool.ActionsFromContext(ctx); actions != nil {
			actions.SkipSummarization = true
		}
	}

	inputText, ok := args["request"].(string)
	if !ok {
		if len(args) > 0 {
			jsonData, err := json.Marshal(args)
			if err != nil {
				return nil, fmt.Errorf("agenttool %s: serialize args: %w", t.wrappedAgent.Name(), err)
			}
			inputText = string(jsonData)
		} else {
			inputText = ""
		}
	}

	sessionService := session.InMemoryService()

	r, err := runner.New(runner.Config{
		AppName:         t.wrappedAgent.Name(),
		Agent:           t.wrappedAgent,
		SessionService:  sessionService,
		ArtifactService: artifact.InMemoryService(),
		MemoryService:   memory.InMemoryService(),
	})
	if err != nil {
		return nil, fmt.Errorf("agenttool %s: create runner: %w", t.wrappedAgent.Name(), err)
	}

	subSession, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName:   t.wrappedAgent.Name(),
		UserID:    "agenttool",
		SessionID: fmt.Sprintf("sub-%d", agenttoolSessionCounter.Add(1)),
		State:     map[string]any{},
	})
	if err != nil {
		return nil, fmt.Errorf("agenttool %s: create session: %w", t.wrappedAgent.Name(), err)
	}

	userMsg := content.NewTextContent(inputText, content.RoleUser)

	var lastEvent *session.Event
	for event, err := range r.Run(ctx, subSession.Session.UserID(), subSession.Session.ID(), userMsg, agent.RunConfig{}) {
		if err != nil {
			return nil, fmt.Errorf("agenttool %s: sub-agent error: %w", t.wrappedAgent.Name(), err)
		}
		if event.ModelResponse != nil {
			lastEvent = event
		}
	}

	if lastEvent == nil {
		return map[string]any{}, nil
	}

	lastContent := lastEvent.ModelResponse
	var textParts []string
	for _, part := range lastContent.Parts {
		if part.Text != nil && part.Text.Text != "" {
			textParts = append(textParts, part.Text.Text)
		}
	}
	outputText := strings.Join(textParts, "\n")

	if outputText == "" {
		return map[string]any{}, nil
	}

	return map[string]any{"result": outputText}, nil
}

var agenttoolSessionCounter atomic.Int64

var _ tool.Tool = (*agentTool)(nil)
