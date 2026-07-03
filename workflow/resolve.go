package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/agent/loopagent"
	"github.com/codeownersnet/goa/agent/sequentialagent"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/skill"
	"github.com/codeownersnet/goa/skill/skilltool"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/exitlooptool"
	"github.com/codeownersnet/goa/tool/mcptoolset"
	toolregistry "github.com/codeownersnet/goa/tool/registry"
)

type resolverConfig struct {
	providerRegistry *provider.Registry
	toolRegistry     *toolregistry.Registry
	skillRegistry    *skill.Registry
	mcpResolver      *mcpResolver
	modelOverride    string
	insideComposite  bool
	allowedPaths     []string
}

func resolve(ctx context.Context, raw *workflowYAML, cfg resolverConfig) (agent.Agent, ExitCondition, []*mcptoolset.Toolset, error) {
	if len(raw.AllowedPaths) > 0 {
		merged := make([]string, len(cfg.allowedPaths), len(cfg.allowedPaths)+len(raw.AllowedPaths))
		copy(merged, cfg.allowedPaths)
		cwd, _ := os.Getwd()
		for _, p := range raw.AllowedPaths {
			abs := p
			if !filepath.IsAbs(p) {
				abs = filepath.Join(cwd, p)
			}
			merged = append(merged, abs)
		}
		cfg.allowedPaths = merged
		cfg.toolRegistry = toolregistry.DefaultBuiltinRegistry(
			toolregistry.WithBuiltinAllowedPaths(merged),
		)
	}

	if cfg.providerRegistry != nil && len(raw.Providers) > 0 {
		for id, p := range raw.Providers {
			provType := p.Type
			if provType == "" {
				provType = "openai_compat"
			}
			envVars := p.Env
			if len(envVars) == 0 {
				envVars = []string{strings.ToUpper(id) + "_API_KEY"}
			}
			cfg.providerRegistry.RegisterProvider(id, &provider.ProviderInfo{
				ID:      id,
				Name:    id,
				APIBase: p.APIBase,
				EnvVars: envVars,
				Type:    provType,
			})
		}
	}

	mcpRes, err := resolveMCPServers(ctx, raw.MCPServers)
	if err != nil {
		return nil, nil, nil, err
	}
	cfg.mcpResolver = mcpRes

	steps, err := resolveSteps(ctx, raw.Steps, cfg, "steps")
	if err != nil {
		_ = mcpRes.closeAll()
		return nil, nil, nil, err
	}

	exitCond, err := newExitCondition(raw.ExitWhen)
	if err != nil {
		_ = mcpRes.closeAll()
		return nil, nil, nil, err
	}

	var ag agent.Agent
	switch raw.Type {
	case wfTypeLoop:
		ag, err = loopagent.New(loopagent.Config{
			Name:          raw.Name,
			Description:   raw.Description,
			SubAgents:     steps,
			MaxIterations: raw.MaxIterations,
		})
	default:
		ag, err = sequentialagent.New(sequentialagent.Config{
			Name:        raw.Name,
			Description: raw.Description,
			SubAgents:   steps,
		})
	}
	if err != nil {
		_ = mcpRes.closeAll()
		return nil, nil, nil, fmt.Errorf("create root agent: %w", err)
	}

	return ag, exitCond, mcpRes.allToolsets(), nil
}

func resolveSteps(ctx context.Context, yamlSteps []stepYAML, cfg resolverConfig, parentPath string) ([]agent.Agent, error) {
	var agents []agent.Agent
	var errs []error

	for i, s := range yamlSteps {
		path := fmt.Sprintf("%s[%d]", parentPath, i)
		switch s.Type {
		case stepTypeLLM:
			a, err := resolveLLMStep(ctx, s, cfg, path)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			agents = append(agents, a)
		case stepTypeLoop:
			a, err := resolveLoopStep(ctx, s, cfg, path)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			agents = append(agents, a)
		case stepTypeSequential:
			a, err := resolveSequentialStep(ctx, s, cfg, path)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			agents = append(agents, a)
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return agents, nil
}

func resolveLLMStep(ctx context.Context, s stepYAML, cfg resolverConfig, path string) (agent.Agent, error) {
	if cfg.providerRegistry == nil {
		return nil, &WorkflowError{Field: path + ".model", Message: "provider registry is required for llm steps"}
	}

	modelStr := s.Model
	if cfg.modelOverride != "" {
		modelStr = cfg.modelOverride
	}

	mdl, err := cfg.providerRegistry.Resolve(ctx, modelStr)
	if err != nil {
		return nil, &WorkflowError{Field: path + ".model", Message: err.Error(), Err: err}
	}

	var tools []tool.Tool
	for _, toolName := range s.Tools {
		if isMCPWildcard(toolName) {
			serverName := mcpServerName(toolName)
			if cfg.mcpResolver == nil {
				return nil, &WorkflowError{
					Field:   fmt.Sprintf("%s.tools[%q]", path, toolName),
					Message: fmt.Sprintf("mcp server %q not declared in mcp_servers", serverName),
				}
			}
			ts, ok := cfg.mcpResolver.toolset(serverName)
			if !ok {
				return nil, &WorkflowError{
					Field:   fmt.Sprintf("%s.tools[%q]", path, toolName),
					Message: fmt.Sprintf("mcp server %q not declared in mcp_servers", serverName),
				}
			}
			tools = append(tools, ts.Tools()...)
			continue
		}

		if cfg.mcpResolver != nil {
			if t, ok := cfg.mcpResolver.lookupTool(toolName); ok {
				tools = append(tools, t)
				continue
			}
		}

		t, err := cfg.toolRegistry.Lookup(toolName)
		if err != nil {
			return nil, &WorkflowError{
				Field:   fmt.Sprintf("%s.tools[%q]", path, toolName),
				Message: err.Error(),
				Err:     err,
			}
		}
		tools = append(tools, t)
	}

	autoSkills := true
	if s.AutoSkills != nil {
		autoSkills = *s.AutoSkills
	}

	if cfg.skillRegistry != nil && autoSkills {
		tools = append(tools, skilltool.NewActivateTool(cfg.skillRegistry))
		tools = append(tools, skilltool.NewResourceTool(cfg.skillRegistry))
	}

	if cfg.insideComposite && !hasToolNamed(tools, "exit_loop") {
		et, err := exitlooptool.New()
		if err != nil {
			return nil, &WorkflowError{Field: path, Message: err.Error(), Err: err}
		}
		tools = append(tools, et)
	}

	instruction := s.Instruction

	if cfg.skillRegistry != nil && len(s.Skills) > 0 {
		var skillParts []string
		for _, skillName := range s.Skills {
			sk, err := cfg.skillRegistry.Activate(skillName)
			if err != nil {
				return nil, &WorkflowError{
					Field:   fmt.Sprintf("%s.skills[%q]", path, skillName),
					Message: err.Error(),
					Err:     err,
				}
			}
			skillParts = append(skillParts, fmt.Sprintf("<skill name=%q>\n%s\n</skill>", sk.Name, sk.Body))
			for _, allowedTool := range sk.AllowedTools {
				if cfg.toolRegistry != nil && cfg.toolRegistry.Has(allowedTool) {
					t, _ := cfg.toolRegistry.Lookup(allowedTool)
					tools = append(tools, t)
				}
			}
		}
		instruction = instruction + "\n\n" + strings.Join(skillParts, "\n\n")
	}

	genCfg := resolveGenerateConfig(s.GenerateConfig)

	noQuestions := true
	if s.NoQuestions != nil {
		noQuestions = *s.NoQuestions
	}

	a, err := llmagent.New(llmagent.Config{
		Name:           s.Name,
		Description:    s.Name,
		Model:          mdl,
		Instruction:    instruction,
		Tools:          tools,
		GenerateConfig: genCfg,
		RequireToolUse: noQuestions,
	})
	if err != nil {
		return nil, &WorkflowError{Field: path, Message: err.Error(), Err: err}
	}
	return a, nil
}

func resolveLoopStep(ctx context.Context, s stepYAML, cfg resolverConfig, path string) (agent.Agent, error) {
	loopCfg := cfg
	loopCfg.insideComposite = true
	subSteps, err := resolveSteps(ctx, s.Steps, loopCfg, path+".steps")
	if err != nil {
		return nil, err
	}

	a, err := loopagent.New(loopagent.Config{
		Name:          s.Name,
		Description:   s.Name,
		SubAgents:     subSteps,
		MaxIterations: s.MaxIterations,
	})
	if err != nil {
		return nil, &WorkflowError{Field: path, Message: err.Error(), Err: err}
	}
	return a, nil
}

func resolveSequentialStep(ctx context.Context, s stepYAML, cfg resolverConfig, path string) (agent.Agent, error) {
	seqCfg := cfg
	seqCfg.insideComposite = true
	subSteps, err := resolveSteps(ctx, s.Steps, seqCfg, path+".steps")
	if err != nil {
		return nil, err
	}

	a, err := sequentialagent.New(sequentialagent.Config{
		Name:        s.Name,
		Description: s.Name,
		SubAgents:   subSteps,
	})
	if err != nil {
		return nil, &WorkflowError{Field: path, Message: err.Error(), Err: err}
	}
	return a, nil
}

func resolveGenerateConfig(raw *generateConfigYAML) *model.GenerateConfig {
	if raw == nil {
		return nil
	}
	cfg := &model.GenerateConfig{
		Temperature:   raw.Temperature,
		TopP:          raw.TopP,
		MaxTokens:     raw.MaxTokens,
		StopSequences: raw.StopSequences,
	}
	if raw.ToolChoice != "" {
		cfg.ToolChoice = model.ToolChoice{Mode: model.ToolChoiceMode(raw.ToolChoice)}
	}
	return cfg
}

func hasToolNamed(tools []tool.Tool, name string) bool {
	for _, t := range tools {
		if t.Name() == name {
			return true
		}
	}
	return false
}
