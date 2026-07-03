package mcptoolset

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	Name           string
	Command        *exec.Cmd
	URL            string
	Headers        map[string]string
	Env            []string
	ConnectTimeout time.Duration
	ToolTimeout    time.Duration
}

type Option func(*Config)

func WithName(name string) Option {
	return func(c *Config) {
		c.Name = name
	}
}

func WithCommand(cmd *exec.Cmd) Option {
	return func(c *Config) {
		c.Command = cmd
	}
}

func WithURL(url string) Option {
	return func(c *Config) {
		c.URL = url
	}
}

func WithHeaders(headers map[string]string) Option {
	return func(c *Config) {
		c.Headers = headers
	}
}

func WithEnv(env []string) Option {
	return func(c *Config) {
		c.Env = env
	}
}

func WithConnectTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.ConnectTimeout = d
	}
}

func WithToolTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.ToolTimeout = d
	}
}

type Toolset struct {
	name        string
	client      *mcp.Client
	session     *mcp.ClientSession
	tools       []tool.Tool
	resources   []tool.ResourceInfo
	prompts     []tool.PromptInfo
	toolTimeout time.Duration
}

func New(ctx context.Context, opts ...Option) (*Toolset, error) {
	cfg := &Config{
		ConnectTimeout: 30 * time.Second,
		ToolTimeout:    60 * time.Second,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.Name == "" {
		cfg.Name = "mcp"
	}

	connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "goa",
		Version: "0.1.0",
	}, &mcp.ClientOptions{
		ToolListChangedHandler: func(_ context.Context, _ *mcp.ToolListChangedRequest) {
		},
		PromptListChangedHandler: func(_ context.Context, _ *mcp.PromptListChangedRequest) {
		},
		ResourceListChangedHandler: func(_ context.Context, _ *mcp.ResourceListChangedRequest) {
		},
	})

	transport, err := newTransport(&transportConfig{
		command: cfg.Command,
		url:     cfg.URL,
		headers: cfg.Headers,
		env:     cfg.Env,
		timeout: cfg.ConnectTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("mcptoolset %q: create transport: %w", cfg.Name, err)
	}

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcptoolset %q: connect: %w", cfg.Name, err)
	}

	ts := &Toolset{
		name:        cfg.Name,
		client:      client,
		session:     session,
		toolTimeout: cfg.ToolTimeout,
	}

	toolTimeoutMs := int(cfg.ToolTimeout.Milliseconds())

	if err := ts.discoverTools(ctx, toolTimeoutMs); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("mcptoolset %q: discover tools: %w", cfg.Name, err)
	}

	if err := ts.discoverResources(ctx); err != nil {
	}

	if err := ts.discoverPrompts(ctx); err != nil {
	}

	return ts, nil
}

func (ts *Toolset) Tools() []tool.Tool {
	return ts.tools
}

func (ts *Toolset) Close() error {
	if ts.session != nil {
		return ts.session.Close()
	}
	return nil
}

func (ts *Toolset) discoverTools(ctx context.Context, toolTimeoutMs int) error {
	var tools []tool.Tool
	for mcpToolDef, err := range ts.session.Tools(ctx, nil) {
		if err != nil {
			return fmt.Errorf("list tools: %w", err)
		}
		t := newMCPTool(ts.name, mcpToolDef, ts.session, toolTimeoutMs)
		tools = append(tools, t)
	}
	ts.tools = tools
	return nil
}

var _ tool.Toolset = (*Toolset)(nil)
