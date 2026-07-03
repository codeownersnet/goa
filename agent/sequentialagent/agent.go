package sequentialagent

import (
	"fmt"
	"iter"
	"strings"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/session"
)

type Config struct {
	Name        string
	Description string
	SubAgents   []agent.Agent

	BeforeAgentCallbacks []agent.BeforeAgentCallback
	AfterAgentCallbacks  []agent.AfterAgentCallback
}

func New(cfg Config) (*Agent, error) {
	if len(cfg.SubAgents) == 0 {
		return nil, fmt.Errorf("sequentialagent: at least one sub-agent is required")
	}

	a := &Agent{}

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
}

func (a *Agent) run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		subAgents := a.SubAgents()
		for i, sub := range subAgents {
			var userContent *content.Content
			if i == 0 {
				userContent = ctx.UserContent()
			}

			subCtx := &sequentialContext{
				InvocationContext: ctx,
				userContent:       userContent,
				branch:            sub.Name(),
			}

			escalated := false
			for ev, err := range sub.Run(subCtx) {
				if err != nil {
					if !yield(nil, fmt.Errorf("sequentialagent %s: sub-agent %q: %w", a.Name(), sub.Name(), err)) {
						return
					}
					return
				}
				if ev != nil && ev.Actions.Escalate {
					escalated = true
				}
				if ev != nil && !ev.Partial && ev.ModelResponse != nil {
					var sb strings.Builder
					for _, part := range ev.ModelResponse.Parts {
						if part.Text != nil {
							sb.WriteString(part.Text.Text)
						}
					}
					if sb.Len() > 0 {
						if i < len(subAgents)-1 {
							nextCtx := &sequentialContext{
								InvocationContext: ctx,
								userContent:       content.NewTextContent(sb.String(), content.RoleUser),
								branch:            subAgents[i+1].Name(),
							}
							ctx = nextCtx
						}
					}
				}
				if !yield(ev, err) {
					return
				}
			}

			if ctx.Ended() || escalated {
				return
			}
		}
	}
}

type sequentialContext struct {
	agent.InvocationContext
	userContent *content.Content
	branch      string
}

func (c *sequentialContext) UserContent() *content.Content {
	return c.userContent
}

func (c *sequentialContext) Branch() string {
	return c.branch
}

func (a *Agent) FindAgent(name string) agent.Agent {
	if a.Name() == name {
		return a
	}
	return a.Agent.FindSubAgent(name)
}

var _ agent.Agent = (*Agent)(nil)
