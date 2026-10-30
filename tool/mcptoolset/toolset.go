package mcptoolset

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	Name                      string
	Command                   *exec.Cmd
	URL                       string
	Headers                   map[string]string
	Env                       []string
	ConnectTimeout            time.Duration
	ToolTimeout               time.Duration
	HTTPClient                *http.Client
	ProtocolVersion           string
	MaxEventSize              int
	MaxRetries                int
	DisableStandaloneSSE      bool
	KeepAlive                 time.Duration
	KeepAliveFailureThreshold int
	Transport                 mcp.Transport
	ElicitationHandler        func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
	ProgressHandler           func(context.Context, *mcp.ProgressNotificationClientRequest)
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

func WithTransport(transport mcp.Transport) Option {
	return func(c *Config) {
		c.Transport = transport
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

func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = client
	}
}

func WithProtocolVersion(version string) Option {
	return func(c *Config) {
		c.ProtocolVersion = version
	}
}

func WithMaxEventSize(size int) Option {
	return func(c *Config) {
		c.MaxEventSize = size
	}
}

func WithMaxRetries(retries int) Option {
	return func(c *Config) {
		c.MaxRetries = retries
	}
}

func WithDisableStandaloneSSE(disable bool) Option {
	return func(c *Config) {
		c.DisableStandaloneSSE = disable
	}
}

func WithKeepAlive(d time.Duration) Option {
	return func(c *Config) {
		c.KeepAlive = d
	}
}

func WithKeepAliveFailureThreshold(threshold int) Option {
	return func(c *Config) {
		c.KeepAliveFailureThreshold = threshold
	}
}

func WithElicitationHandler(handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) Option {
	return func(c *Config) {
		c.ElicitationHandler = handler
	}
}

func WithProgressHandler(handler func(context.Context, *mcp.ProgressNotificationClientRequest)) Option {
	return func(c *Config) {
		c.ProgressHandler = handler
	}
}

type Toolset struct {
	name              string
	client            *mcp.Client
	session           *mcp.ClientSession
	tools             []tool.Tool
	resources         []tool.ResourceInfo
	resourceTemplates []tool.ResourceTemplateInfo
	prompts           []tool.PromptInfo
	toolTimeout       time.Duration

	mu        sync.RWMutex
	refreshMu sync.Mutex
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

	ts := &Toolset{
		name:        cfg.Name,
		toolTimeout: cfg.ToolTimeout,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "goa",
		Version: "0.1.0",
	}, &mcp.ClientOptions{
		ToolListChangedHandler: func(ctx context.Context, _ *mcp.ToolListChangedRequest) {
			ts.refreshTools(ctx)
		},
		PromptListChangedHandler: func(ctx context.Context, _ *mcp.PromptListChangedRequest) {
			ts.refreshPrompts(ctx)
		},
		ResourceListChangedHandler: func(ctx context.Context, _ *mcp.ResourceListChangedRequest) {
			ts.refreshResources(ctx)
		},
		KeepAlive:                   cfg.KeepAlive,
		KeepAliveFailureThreshold:   cfg.KeepAliveFailureThreshold,
		ElicitationHandler:          cfg.ElicitationHandler,
		ProgressNotificationHandler: cfg.ProgressHandler,
	})
	ts.client = client

	transport := cfg.Transport
	if transport == nil {
		var err error
		transport, err = newTransport(&transportConfig{
			command:              cfg.Command,
			url:                  cfg.URL,
			headers:              cfg.Headers,
			env:                  cfg.Env,
			httpClient:           cfg.HTTPClient,
			maxEventSize:         cfg.MaxEventSize,
			maxRetries:           cfg.MaxRetries,
			disableStandaloneSSE: cfg.DisableStandaloneSSE,
		})
		if err != nil {
			return nil, fmt.Errorf("mcptoolset %q: create transport: %w", cfg.Name, err)
		}
	}

	var sessionOpts *mcp.ClientSessionOptions
	if cfg.ProtocolVersion != "" {
		sessionOpts = &mcp.ClientSessionOptions{ProtocolVersion: cfg.ProtocolVersion}
	}

	session, err := client.Connect(connectCtx, transport, sessionOpts)
	if err != nil {
		return nil, fmt.Errorf("mcptoolset %q: connect: %w", cfg.Name, err)
	}

	ts.mu.Lock()
	ts.session = session
	ts.mu.Unlock()

	if err := ts.discoverTools(ctx); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("mcptoolset %q: discover tools: %w", cfg.Name, err)
	}

	if err := ts.discoverResources(ctx); err != nil && !isMethodNotFound(err) {
		_ = session.Close()
		return nil, fmt.Errorf("mcptoolset %q: discover resources: %w", cfg.Name, err)
	}

	if err := ts.discoverResourceTemplates(ctx); err != nil && !isMethodNotFound(err) {
		_ = session.Close()
		return nil, fmt.Errorf("mcptoolset %q: discover resource templates: %w", cfg.Name, err)
	}

	if err := ts.discoverPrompts(ctx); err != nil && !isMethodNotFound(err) {
		_ = session.Close()
		return nil, fmt.Errorf("mcptoolset %q: discover prompts: %w", cfg.Name, err)
	}

	return ts, nil
}

func (ts *Toolset) Tools() []tool.Tool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return append([]tool.Tool(nil), ts.tools...)
}

func (ts *Toolset) Close() error {
	session := ts.currentSession()
	if session != nil {
		return session.Close()
	}
	return nil
}

func (ts *Toolset) ProtocolVersion() string {
	session := ts.currentSession()
	if session == nil {
		return ""
	}
	if res := session.InitializeResult(); res != nil {
		return res.ProtocolVersion
	}
	return ""
}

func (ts *Toolset) ServerInfo() *mcp.Implementation {
	session := ts.currentSession()
	if session == nil {
		return nil
	}
	if res := session.InitializeResult(); res != nil {
		return res.ServerInfo
	}
	return nil
}

func (ts *Toolset) currentSession() *mcp.ClientSession {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.session
}

func (ts *Toolset) discoverTools(ctx context.Context) error {
	session := ts.currentSession()
	if session == nil {
		return errNotConnected
	}

	var tools []tool.Tool
	for mcpToolDef, err := range session.Tools(ctx, nil) {
		if err != nil {
			return fmt.Errorf("list tools: %w", err)
		}
		t := newMCPTool(ts.name, mcpToolDef, session, ts.toolTimeout)
		tools = append(tools, t)
	}

	ts.mu.Lock()
	ts.tools = tools
	ts.mu.Unlock()
	return nil
}

func (ts *Toolset) refreshTools(ctx context.Context) {
	ts.refreshMu.Lock()
	defer ts.refreshMu.Unlock()
	ctx, cancel := ts.refreshContext(ctx)
	defer cancel()
	_ = ts.discoverTools(ctx)
}

func (ts *Toolset) refreshResources(ctx context.Context) {
	ts.refreshMu.Lock()
	defer ts.refreshMu.Unlock()
	ctx, cancel := ts.refreshContext(ctx)
	defer cancel()
	_ = ts.discoverResources(ctx)
	_ = ts.discoverResourceTemplates(ctx)
}

func (ts *Toolset) refreshPrompts(ctx context.Context) {
	ts.refreshMu.Lock()
	defer ts.refreshMu.Unlock()
	ctx, cancel := ts.refreshContext(ctx)
	defer cancel()
	_ = ts.discoverPrompts(ctx)
}

func (ts *Toolset) refreshContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx = context.WithoutCancel(ctx)
	if ts.toolTimeout > 0 {
		return context.WithTimeout(ctx, ts.toolTimeout)
	}
	return context.WithCancel(ctx)
}

func isMethodNotFound(err error) bool {
	var rpcErr *jsonrpc.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == jsonrpc.CodeMethodNotFound
}

var errNotConnected = errors.New("mcptoolset: not connected")

var _ tool.Toolset = (*Toolset)(nil)
