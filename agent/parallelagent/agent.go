package parallelagent

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"github.com/codeownersnet/goa/agent"
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
		return nil, fmt.Errorf("parallelagent: at least one sub-agent is required")
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
		type agentResult struct {
			events []*session.Event
			err    error
			name   string
		}

		subAgents := a.SubAgents()
		results := make([]agentResult, len(subAgents))
		var wg sync.WaitGroup
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for i, sub := range subAgents {
			wg.Add(1)
			go func(idx int, sa agent.Agent) {
				defer wg.Done()
				parCtx := &parallelContext{
					InvocationContext: ctx.WithContext(runCtx),
					branch:            fmt.Sprintf("%s/parallel/%s", a.Name(), sa.Name()),
				}

				var collected []*session.Event
				for ev, err := range sa.Run(parCtx) {
					if err != nil {
						results[idx] = agentResult{err: fmt.Errorf("parallelagent %s: sub-agent %q: %w", a.Name(), sa.Name(), err), name: sa.Name()}
						cancel()
						return
					}
					if ev != nil {
						collected = append(collected, ev)
					}
				}
				results[idx] = agentResult{events: collected, name: sa.Name()}
			}(i, sub)
		}

		wg.Wait()

		for _, res := range results {
			if res.err != nil {
				if !yield(nil, res.err) {
					return
				}
				return
			}
			for _, ev := range res.events {
				if !yield(ev, nil) {
					return
				}
			}
		}
	}
}

type parallelContext struct {
	agent.InvocationContext
	branch string
}

func (c *parallelContext) Branch() string {
	return c.branch
}

func (a *Agent) FindAgent(name string) agent.Agent {
	if a.Name() == name {
		return a
	}
	return a.Agent.FindSubAgent(name)
}

var _ agent.Agent = (*Agent)(nil)
