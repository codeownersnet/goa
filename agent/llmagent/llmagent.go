package llmagent

import (
	"fmt"
	"iter"
	"strings"

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

	Model             model.Model
	Instruction       string
	Tools             []tool.Tool
	GenerateConfig    *model.GenerateConfig
	RequestProcessors []flow.RequestProcessor
	RequireToolUse    bool

	BeforeAgentCallbacks []agent.BeforeAgentCallback
	AfterAgentCallbacks  []agent.AfterAgentCallback
}

func New(cfg Config) (*Agent, error) {
	a := &Agent{
		model:             cfg.Model,
		instruction:       cfg.Instruction,
		tools:             cfg.Tools,
		generateConfig:    cfg.GenerateConfig,
		requestProcessors: cfg.RequestProcessors,
		requireToolUse:    cfg.RequireToolUse,
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

	model             model.Model
	instruction       string
	tools             []tool.Tool
	generateConfig    *model.GenerateConfig
	requestProcessors []flow.RequestProcessor
	requireToolUse    bool
}

func (a *Agent) run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	f := &flow.Flow{
		Model:             a.model,
		Tools:             a.tools,
		Instruction:       a.instruction,
		GenerateConfig:    a.generateConfig,
		RequestProcessors: a.requestProcessors,
		RequireToolUse:    a.requireToolUse,
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
