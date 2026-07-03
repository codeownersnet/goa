package workflow

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/mcptoolset"
)

type mcpResolver struct {
	toolsets map[string]*mcptoolset.Toolset
}

func (mr *mcpResolver) lookupTool(name string) (tool.Tool, bool) {
	for _, ts := range mr.toolsets {
		for _, t := range ts.Tools() {
			if t.Name() == name {
				return t, true
			}
		}
	}
	return nil, false
}

func (mr *mcpResolver) toolset(name string) (*mcptoolset.Toolset, bool) {
	ts, ok := mr.toolsets[name]
	return ts, ok
}

func (mr *mcpResolver) allToolsets() []*mcptoolset.Toolset {
	all := make([]*mcptoolset.Toolset, 0, len(mr.toolsets))
	for _, ts := range mr.toolsets {
		all = append(all, ts)
	}
	return all
}

func (mr *mcpResolver) closeAll() error {
	var errs []error
	for _, ts := range mr.toolsets {
		if err := ts.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close mcp toolsets: %w", joinErrors(errs))
	}
	return nil
}

func isMCPWildcard(name string) bool {
	return strings.HasPrefix(name, "mcp:") && len(name) > 4
}

func mcpServerName(name string) string {
	return strings.TrimPrefix(name, "mcp:")
}

func resolveMCPServers(ctx context.Context, servers map[string]mcpServerYAML) (*mcpResolver, error) {
	resolver := &mcpResolver{toolsets: make(map[string]*mcptoolset.Toolset, len(servers))}
	if len(servers) == 0 {
		return resolver, nil
	}

	for name, cfg := range servers {
		opts, err := mcpServerYAMLOptions(name, cfg)
		if err != nil {
			_ = resolver.closeAll()
			return nil, &WorkflowError{Field: "mcp_servers." + name, Message: err.Error(), Err: err}
		}
		ts, err := mcptoolset.New(ctx, opts...)
		if err != nil {
			_ = resolver.closeAll()
			return nil, &WorkflowError{Field: "mcp_servers." + name, Message: err.Error(), Err: err}
		}
		resolver.toolsets[name] = ts
	}
	return resolver, nil
}

func mcpServerYAMLOptions(name string, cfg mcpServerYAML) ([]mcptoolset.Option, error) {
	var opts []mcptoolset.Option
	opts = append(opts, mcptoolset.WithName(name))

	if cfg.Command != "" {
		opts = append(opts, mcptoolset.WithCommand(exec.Command("sh", "-c", cfg.Command)))
	} else if cfg.URL != "" {
		opts = append(opts, mcptoolset.WithURL(cfg.URL))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, mcptoolset.WithHeaders(cfg.Headers))
	}
	if len(cfg.Env) > 0 {
		opts = append(opts, mcptoolset.WithEnv(cfg.Env))
	}
	if cfg.ConnectTimeout != "" {
		d, err := time.ParseDuration(cfg.ConnectTimeout)
		if err != nil {
			return nil, fmt.Errorf("parse connect_timeout: %w", err)
		}
		opts = append(opts, mcptoolset.WithConnectTimeout(d))
	}
	if cfg.ToolTimeout != "" {
		d, err := time.ParseDuration(cfg.ToolTimeout)
		if err != nil {
			return nil, fmt.Errorf("parse tool_timeout: %w", err)
		}
		opts = append(opts, mcptoolset.WithToolTimeout(d))
	}
	return opts, nil
}
