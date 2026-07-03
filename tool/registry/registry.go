package registry

import (
	"fmt"
	"sync"

	"github.com/codeownersnet/goa/tool"
)

type ToolFactory func() (tool.Tool, error)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]ToolFactory
}

type Option func(*Registry)

func WithFactory(name string, factory ToolFactory) Option {
	return func(r *Registry) {
		r.factories[name] = factory
	}
}

func New(opts ...Option) *Registry {
	r := &Registry{
		factories: make(map[string]ToolFactory),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Registry) Register(name string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

func (r *Registry) Lookup(name string) (tool.Tool, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("tool %q: %w", name, ErrToolNotFound)
	}
	t, err := factory()
	if err != nil {
		return nil, fmt.Errorf("tool %q: create: %w", name, err)
	}
	return t, nil
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for k := range r.factories {
		names = append(names, k)
	}
	return names
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}

var ErrToolNotFound = fmt.Errorf("tool not found in registry")
