package parentmap

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/agent"
)

func New(root agent.Agent) (Map, error) {
	res := make(map[string]agent.Agent)
	rootName := root.Name()
	pointerMap := map[agent.Agent]string{root: "root"}

	var walk func(cur agent.Agent) error
	walk = func(cur agent.Agent) error {
		for _, sub := range cur.SubAgents() {
			if p, ok := pointerMap[sub]; ok {
				return fmt.Errorf("%q agent cannot have multiple parents, found: %q and %q", sub.Name(), p, cur.Name())
			}
			if _, ok := res[sub.Name()]; ok || sub.Name() == rootName {
				return fmt.Errorf("duplicate agent name in tree: %q", sub.Name())
			}
			res[sub.Name()] = cur
			pointerMap[sub] = cur.Name()
			if err := walk(sub); err != nil {
				return err
			}
		}
		return nil
	}

	return res, walk(root)
}

func (m Map) RootAgent(cur agent.Agent) agent.Agent {
	if cur == nil {
		return nil
	}
	for {
		parent := m[cur.Name()]
		if parent == nil {
			return cur
		}
		cur = parent
	}
}

func ToContext(ctx context.Context, parents Map) context.Context {
	return context.WithValue(ctx, mapCtxKey, parents)
}

func FromContext(ctx context.Context) Map {
	m, ok := ctx.Value(mapCtxKey).(Map)
	if !ok {
		return nil
	}
	return m
}

type ctxKey int

const mapCtxKey ctxKey = 0

type Map map[string]agent.Agent
