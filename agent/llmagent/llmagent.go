package llmagent

import (
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/flow"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
)

type Config struct {
	Name        string
	Description string
	SubAgents   []agent.Agent

	Model              model.Model
	Instruction        string
	Tools              []tool.Tool
	Toolsets           []tool.Toolset
	GenerateConfig     *model.GenerateConfig
	MaxIterations      int
	ToolTimeout        time.Duration
	RequestProcessors  []flow.RequestProcessor
	ResponseProcessors []func(agent.InvocationContext, *model.ModelRequest, *model.ModelResponse) error
	ResultPolicy       flow.ResultPolicy
	ExecutionOptions   flow.ExecutionOptions
	RequireToolUse     bool

	BeforeAgentCallbacks []agent.BeforeAgentCallback
	AfterAgentCallbacks  []agent.AfterAgentCallback

	BeforeModelCallbacks  []flow.BeforeModelCallback
	AfterModelCallbacks   []flow.AfterModelCallback
	OnModelErrorCallbacks []flow.OnModelErrorCallback
	BeforeToolCallbacks   []flow.BeforeToolCallback
	AfterToolCallbacks    []flow.AfterToolCallback
	OnToolErrorCallbacks  []flow.OnToolErrorCallback
}

func New(cfg Config) (*Agent, error) {
	a := &Agent{
		model:                 cfg.Model,
		instruction:           cfg.Instruction,
		tools:                 cfg.Tools,
		toolsets:              cfg.Toolsets,
		generateConfig:        cfg.GenerateConfig,
		maxIterations:         cfg.MaxIterations,
		toolTimeout:           cfg.ToolTimeout,
		requestProcessors:     cfg.RequestProcessors,
		responseProcessors:    cfg.ResponseProcessors,
		resultPolicy:          cfg.ResultPolicy,
		executionOptions:      cfg.ExecutionOptions,
		requireToolUse:        cfg.RequireToolUse,
		beforeModelCallbacks:  cfg.BeforeModelCallbacks,
		afterModelCallbacks:   cfg.AfterModelCallbacks,
		onModelErrorCallbacks: cfg.OnModelErrorCallbacks,
		beforeToolCallbacks:   cfg.BeforeToolCallbacks,
		afterToolCallbacks:    cfg.AfterToolCallbacks,
		onToolErrorCallbacks:  cfg.OnToolErrorCallbacks,
	}

	base := agent.New(
		agent.WithName(cfg.Name),
		agent.WithDescription(cfg.Description),
		agent.WithSubAgents(cfg.SubAgents...),
		agent.WithBeforeAgentCallbacks(cfg.BeforeAgentCallbacks...),
		agent.WithAfterAgentCallbacks(cfg.AfterAgentCallbacks...),
		agent.WithRun(a.run),
	)

	a.Agent = base
	return a, nil
}

type Agent struct {
	agent.Agent

	model                 model.Model
	instruction           string
	tools                 []tool.Tool
	toolsets              []tool.Toolset
	generateConfig        *model.GenerateConfig
	maxIterations         int
	toolTimeout           time.Duration
	requestProcessors     []flow.RequestProcessor
	responseProcessors    []func(agent.InvocationContext, *model.ModelRequest, *model.ModelResponse) error
	resultPolicy          flow.ResultPolicy
	executionOptions      flow.ExecutionOptions
	requireToolUse        bool
	beforeModelCallbacks  []flow.BeforeModelCallback
	afterModelCallbacks   []flow.AfterModelCallback
	onModelErrorCallbacks []flow.OnModelErrorCallback
	beforeToolCallbacks   []flow.BeforeToolCallback
	afterToolCallbacks    []flow.AfterToolCallback
	onToolErrorCallbacks  []flow.OnToolErrorCallback
}

func (a *Agent) run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	f := &flow.Flow{
		Model:                 a.model,
		Tools:                 a.tools,
		Toolsets:              a.toolsets,
		Instruction:           a.instruction,
		GenerateConfig:        a.generateConfig,
		MaxIterations:         a.maxIterations,
		ToolTimeout:           a.toolTimeout,
		RequestProcessors:     a.requestProcessors,
		ResponseProcessors:    a.responseProcessors,
		ResultPolicy:          a.resultPolicy,
		ExecutionOptions:      a.executionOptions,
		RequireToolUse:        a.requireToolUse,
		BeforeModelCallbacks:  a.beforeModelCallbacks,
		AfterModelCallbacks:   a.afterModelCallbacks,
		OnModelErrorCallbacks: a.onModelErrorCallbacks,
		BeforeToolCallbacks:   a.beforeToolCallbacks,
		AfterToolCallbacks:    a.afterToolCallbacks,
		OnToolErrorCallbacks:  a.onToolErrorCallbacks,
	}

	return func(yield func(*session.Event, error) bool) {
		for ev, err := range f.Run(ctx) {
			if err != nil {
				if !yield(ev, err) {
					return
				}
				return
			}
			a.maybeSaveOutputToState(ev)

			if ev != nil && ev.Actions.TransferToAgent != "" {
				target := a.FindAgent(ev.Actions.TransferToAgent)
				if target == nil {
					yield(nil, fmt.Errorf("llmagent %s: transfer target %q not found", a.Name(), ev.Actions.TransferToAgent))
					return
				}
				if !yield(ev, nil) {
					return
				}
				transferCtx := &transferContext{
					InvocationContext: ctx,
					userContent:       content.NewTextContent("Transferred from "+a.Name(), content.RoleUser),
					branch:            target.Name(),
				}
				for transferEv, transferErr := range target.Run(transferCtx) {
					if transferErr != nil {
						if !yield(nil, transferErr) {
							return
						}
						return
					}
					if !yield(transferEv, nil) {
						return
					}
				}
				return
			}

			if ev != nil && ev.Actions.Escalate {
				if !yield(ev, nil) {
					return
				}
				return
			}

			if !yield(ev, nil) {
				return
			}
		}
	}
}

type transferContext struct {
	agent.InvocationContext
	userContent *content.Content
	branch      string
}

func (c *transferContext) UserContent() *content.Content {
	return c.userContent
}

func (c *transferContext) Branch() string {
	return c.branch
}

func (a *Agent) maybeSaveOutputToState(event *session.Event) {
	if event == nil {
		return
	}
	if event.Author != a.Name() {
		return
	}
	if !event.Partial && event.ModelResponse != nil {
		var sb strings.Builder
		for _, part := range event.ModelResponse.Parts {
			if part.Text != nil {
				sb.WriteString(part.Text.Text)
			}
		}
		result := sb.String()
		if result == "" {
			return
		}
		if event.Actions.StateDelta == nil {
			event.Actions.StateDelta = make(map[string]any)
		}
		event.Actions.StateDelta["output"] = result
	}
}

func (a *Agent) FindAgent(name string) agent.Agent {
	if a.Name() == name {
		return a
	}
	return a.Agent.FindSubAgent(name)
}

var _ agent.Agent = (*Agent)(nil)
