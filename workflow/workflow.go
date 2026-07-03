package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/skill"
	"github.com/codeownersnet/goa/tool/mcptoolset"
	toolregistry "github.com/codeownersnet/goa/tool/registry"
)

type Workflow struct {
	agent    agent.Agent
	exitCond ExitCondition
	name     string
	desc     string
	toolsets []*mcptoolset.Toolset
}

type LoadOption func(*loadConfig)

type loadConfig struct {
	providerRegistry *provider.Registry
	toolRegistry     *toolregistry.Registry
	skillRegistry    *skill.Registry
	modelOverride    string
	allowedPaths     []string
}

func WithProviderRegistry(r *provider.Registry) LoadOption {
	return func(c *loadConfig) { c.providerRegistry = r }
}

func WithToolRegistry(r *toolregistry.Registry) LoadOption {
	return func(c *loadConfig) { c.toolRegistry = r }
}

func WithSkillRegistry(r *skill.Registry) LoadOption {
	return func(c *loadConfig) { c.skillRegistry = r }
}

func WithModelOverride(s string) LoadOption {
	return func(c *loadConfig) { c.modelOverride = s }
}

func WithAllowedPaths(paths []string) LoadOption {
	return func(c *loadConfig) { c.allowedPaths = paths }
}

func Load(ctx context.Context, path string, opts ...LoadOption) (*Workflow, error) {
	raw, err := parseFile(path)
	if err != nil {
		return nil, err
	}
	return build(ctx, raw, opts...)
}

func LoadFromBytes(ctx context.Context, data []byte, opts ...LoadOption) (*Workflow, error) {
	raw, err := parse(data)
	if err != nil {
		return nil, err
	}
	return build(ctx, raw, opts...)
}

func build(ctx context.Context, raw *workflowYAML, opts ...LoadOption) (*Workflow, error) {
	cfg := loadConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if err := validate(raw); err != nil {
		return nil, err
	}

	ag, exitCond, toolsets, err := resolve(ctx, raw, resolverConfig{
		providerRegistry: cfg.providerRegistry,
		toolRegistry:     cfg.toolRegistry,
		skillRegistry:    cfg.skillRegistry,
		modelOverride:    cfg.modelOverride,
		allowedPaths:     cfg.allowedPaths,
	})
	if err != nil {
		return nil, err
	}

	return &Workflow{
		agent:    ag,
		exitCond: exitCond,
		name:     raw.Name,
		desc:     raw.Description,
		toolsets: toolsets,
	}, nil
}

func (w *Workflow) Agent() agent.Agent {
	return w.agent
}

func (w *Workflow) Name() string {
	return w.name
}

func (w *Workflow) Description() string {
	return w.desc
}

func (w *Workflow) ExitCondition() ExitCondition {
	return w.exitCond
}

func (w *Workflow) Close() error {
	var errs []error
	for _, ts := range w.toolsets {
		if err := ts.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	w.toolsets = nil
	if len(errs) > 0 {
		return fmt.Errorf("close mcp toolsets: %w", errors.Join(errs...))
	}
	return nil
}

func ValidateFile(_ context.Context, path string) error {
	raw, err := parseFile(path)
	if err != nil {
		return err
	}
	return validate(raw)
}

func ValidateBytes(_ context.Context, data []byte) error {
	raw, err := parse(data)
	if err != nil {
		return err
	}
	return validate(raw)
}
