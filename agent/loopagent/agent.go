package loopagent

import (
	"fmt"
	"iter"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/session"
)

type Config struct {
	Name          string
	Description   string
	SubAgents     []agent.Agent
	MaxIterations int

	BeforeAgentCallbacks []agent.BeforeAgentCallback
	AfterAgentCallbacks  []agent.AfterAgentCallback
}

func New(cfg Config) (*Agent, error) {
	if len(cfg.SubAgents) == 0 {
		return nil, fmt.Errorf("loopagent: at least one sub-agent is required")
	}

	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	a := &Agent{
		maxIterations: maxIter,
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
	maxIterations int
}

func (a *Agent) run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		for i := 0; i < a.maxIterations; i++ {
			escalated := false

			for _, sub := range a.SubAgents() {
				loopCtx := &loopContext{
					InvocationContext: ctx,
					iteration:         i,
					branch:            fmt.Sprintf("%s/iter%d/%s", a.Name(), i, sub.Name()),
				}

				for ev, err := range sub.Run(loopCtx) {
					if err != nil {
						if !yield(nil, fmt.Errorf("loopagent %s: iteration %d sub-agent %q: %w", a.Name(), i, sub.Name(), err)) {
							return
						}
						return
					}
					if ev != nil && ev.Actions.Escalate {
						escalated = true
					}
					if !yield(ev, err) {
						return
					}
				}

				if ctx.Ended() || escalated {
					break
				}
			}

			if ctx.Ended() || escalated {
				return
			}
		}
	}
}

type loopContext struct {
	agent.InvocationContext
	iteration int
	branch    string
}

func (c *loopContext) Branch() string {
	return c.branch
}

func (a *Agent) FindAgent(name string) agent.Agent {
	if a.Name() == name {
		return a
	}
	return a.Agent.FindSubAgent(name)
}

var _ agent.Agent = (*Agent)(nil)
