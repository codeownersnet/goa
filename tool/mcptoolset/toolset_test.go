package mcptoolset

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) *mcp.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		CompletionHandler: func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return &mcp.CompleteResult{
				Completion: mcp.CompletionResultDetails{
					Values: []string{"Alice", "Alfred"},
					Total:  2,
				},
			}, nil
		},
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "greet",
		Description: "Greet a person by name",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name string `json:"name" jsonschema:"the person to greet"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Hello, " + args.Name + "!"},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add",
		Description: "Add two numbers",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		A int `json:"a" jsonschema:"first number"`
		B int `json:"b" jsonschema:"second number"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Result: " + any(args.A+args.B).(string)},
			},
		}, nil, nil
	})

	server.AddResource(&mcp.Resource{
		Name:     "readme",
		URI:      "file:///tmp/readme.md",
		MIMEType: "text/markdown",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      "file:///tmp/readme.md",
					MIMEType: "text/markdown",
					Text:     "# Hello\n\nThis is a test resource.",
				},
			},
		}, nil
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "file",
		URITemplate: "file:///{path}",
		Description: "A file resource",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "text/plain", Text: "template"},
			},
		}, nil
	})

	server.AddPrompt(&mcp.Prompt{
		Name:        "greet_prompt",
		Description: "Generate a greeting prompt",
		Arguments: []*mcp.PromptArgument{
			{Name: "name", Description: "the person to greet", Required: true},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		name := "friend"
		if req.Params.Arguments != nil {
			if n, ok := req.Params.Arguments["name"]; ok {
				name = n
			}
		}
		return &mcp.GetPromptResult{
			Description: "Greeting prompt",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: "Please greet " + name}},
			},
		}, nil
	})

	return server
}

func connectTestToolset(t *testing.T) (*Toolset, *mcp.Server) {
	t.Helper()
	server := newTestServer(t)

	t1, t2 := mcp.NewInMemoryTransports()

	_, err := server.Connect(context.Background(), t1, nil)
	require.NoError(t, err)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "goa-test",
		Version: "0.0.1",
	}, nil)

	session, err := client.Connect(context.Background(), t2, nil)
	require.NoError(t, err)

	ts := &Toolset{
		name:    "test",
		client:  client,
		session: session,
	}

	require.NoError(t, ts.discoverTools(context.Background()))
	require.NoError(t, ts.discoverResources(context.Background()))
	require.NoError(t, ts.discoverResourceTemplates(context.Background()))
	require.NoError(t, ts.discoverPrompts(context.Background()))

	t.Cleanup(func() {
		_ = ts.Close()
	})

	return ts, server
}

func TestToolsetTools(t *testing.T) {
	ts, _ := connectTestToolset(t)

	tools := ts.Tools()
	assert.Len(t, tools, 2)

	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name()
	}
	assert.Contains(t, names, "test_greet")
	assert.Contains(t, names, "test_add")
}

func TestToolsetNameNamespacing(t *testing.T) {
	ts, _ := connectTestToolset(t)

	tools := ts.Tools()
	for _, tl := range tools {
		assert.True(t, strings.HasPrefix(tl.Name(), "test_"))
	}
}

func TestToolDescriptions(t *testing.T) {
	ts, _ := connectTestToolset(t)

	tools := ts.Tools()
	var greetTool tool.Tool
	for _, t := range tools {
		if t.Name() == "test_greet" {
			greetTool = t
			break
		}
	}
	require.NotNil(t, greetTool)
	assert.Equal(t, "Greet a person by name", greetTool.Description())
}

func TestToolDeclaration(t *testing.T) {
	ts, _ := connectTestToolset(t)

	tools := ts.Tools()
	var greetTool tool.Tool
	for _, t := range tools {
		if t.Name() == "test_greet" {
			greetTool = t
			break
		}
	}
	require.NotNil(t, greetTool)

	declarer, ok := greetTool.(tool.Declarer)
	require.True(t, ok)

	decl := declarer.Declaration()
	assert.Equal(t, "test_greet", decl.Name)
	assert.NotNil(t, decl.Parameters)
	assert.Equal(t, "object", decl.Parameters.Type)
}

func TestToolProcess(t *testing.T) {
	ts, _ := connectTestToolset(t)

	tools := ts.Tools()
	var greetTool tool.Tool
	for _, t := range tools {
		if t.Name() == "test_greet" {
			greetTool = t
			break
		}
	}
	require.NotNil(t, greetTool)

	result, err := greetTool.Process(context.Background(), map[string]any{"name": "World"})
	require.NoError(t, err)
	assert.Contains(t, result["result"], "Hello, World!")
}

func TestToolsetResources(t *testing.T) {
	ts, _ := connectTestToolset(t)

	resources := ts.Resources()
	assert.Len(t, resources, 1)
	assert.Equal(t, "readme", resources[0].Name)
	assert.Equal(t, "file:///tmp/readme.md", resources[0].URI)
	assert.Equal(t, "text/markdown", resources[0].MimeType)
}

func TestToolsetReadResource(t *testing.T) {
	ts, _ := connectTestToolset(t)

	data, err := ts.ReadResource(context.Background(), "file:///tmp/readme.md")
	require.NoError(t, err)
	assert.Contains(t, string(data), "# Hello")
	assert.Contains(t, string(data), "test resource")
}

func TestToolsetPrompts(t *testing.T) {
	ts, _ := connectTestToolset(t)

	prompts := ts.Prompts()
	assert.Len(t, prompts, 1)
	assert.Equal(t, "greet_prompt", prompts[0].Name)
	assert.Len(t, prompts[0].Arguments, 1)
	assert.Equal(t, "name", prompts[0].Arguments[0].Name)
	assert.True(t, prompts[0].Arguments[0].Required)
}

func TestToolsetGetPrompt(t *testing.T) {
	ts, _ := connectTestToolset(t)

	text, err := ts.GetPrompt(context.Background(), "greet_prompt", map[string]string{"name": "Alice"})
	require.NoError(t, err)
	assert.Contains(t, text, "Please greet Alice")
}

func connectNewToolset(t *testing.T, opts ...Option) (*Toolset, *mcp.Server) {
	t.Helper()
	server := newTestServer(t)

	t1, t2 := mcp.NewInMemoryTransports()
	_, err := server.Connect(context.Background(), t1, nil)
	require.NoError(t, err)

	opts = append(opts, WithName("test"), WithTransport(t2))
	ts, err := New(context.Background(), opts...)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = ts.Close()
	})

	return ts, server
}

func TestToolsetRefreshesOnToolListChanged(t *testing.T) {
	ts, server := connectNewToolset(t)
	require.Len(t, ts.Tools(), 2)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "new_tool",
		Description: "Added after connect",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	require.Eventually(t, func() bool {
		for _, tl := range ts.Tools() {
			if tl.Name() == "test_new_tool" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

func TestToolsetProtocolVersionPinned(t *testing.T) {
	ts, _ := connectNewToolset(t, WithProtocolVersion("2025-11-25"))

	assert.Equal(t, "2025-11-25", ts.ProtocolVersion())
}

func TestToolsetResourceTemplates(t *testing.T) {
	ts, _ := connectTestToolset(t)

	templates := ts.ResourceTemplates()
	require.Len(t, templates, 1)
	assert.Equal(t, "file", templates[0].Name)
	assert.Equal(t, "file:///{path}", templates[0].URITemplate)
	assert.Equal(t, "text/plain", templates[0].MimeType)
}

func TestToolsetComplete(t *testing.T) {
	ts, _ := connectTestToolset(t)

	result, err := ts.Complete(context.Background(), tool.CompletionRequest{
		Ref:      tool.CompletionReference{Type: "ref/prompt", Name: "greet_prompt"},
		Argument: "name",
		Value:    "Al",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Alice", "Alfred"}, result.Values)
	assert.Equal(t, 2, result.Total)
}

func TestToolsetElicitationHandler(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "elicit-server",
		Version: "1.0.0",
	}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "ask",
		Description: "Ask the client for input",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		result, err := req.Session.Elicit(ctx, &mcp.ElicitParams{
			Message: "Your name?",
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		})
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: result.Action}},
		}, nil, nil
	})

	t1, t2 := mcp.NewInMemoryTransports()
	_, err := server.Connect(context.Background(), t1, nil)
	require.NoError(t, err)

	ts, err := New(context.Background(),
		WithName("elicit"),
		WithTransport(t2),
		WithProtocolVersion("2025-11-25"),
		WithElicitationHandler(func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"name": "Alice"}}, nil
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ts.Close() })

	var askTool tool.Tool
	for _, tl := range ts.Tools() {
		if tl.Name() == "elicit_ask" {
			askTool = tl
			break
		}
	}
	require.NotNil(t, askTool)

	result, err := askTool.Process(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "accept", result["result"])
}

func TestToolsetProgressHandler(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "progress-server",
		Version: "1.0.0",
	}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "work",
		Description: "Report progress",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: "token",
			Progress:      1,
			Total:         2,
			Message:       "halfway",
		})
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil, nil
	})

	t1, t2 := mcp.NewInMemoryTransports()
	_, err := server.Connect(context.Background(), t1, nil)
	require.NoError(t, err)

	messages := make(chan string, 1)
	ts, err := New(context.Background(),
		WithName("progress"),
		WithTransport(t2),
		WithProtocolVersion("2025-11-25"),
		WithProgressHandler(func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			messages <- req.Params.Message
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ts.Close() })

	var workTool tool.Tool
	for _, tl := range ts.Tools() {
		if tl.Name() == "progress_work" {
			workTool = tl
			break
		}
	}
	require.NotNil(t, workTool)

	_, err = workTool.Process(context.Background(), nil)
	require.NoError(t, err)

	select {
	case message := <-messages:
		assert.Equal(t, "halfway", message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for progress notification")
	}
}

func TestToolsetDiagnostics(t *testing.T) {
	ts, _ := connectTestToolset(t)

	assert.NotEmpty(t, ts.ProtocolVersion())
	require.NotNil(t, ts.ServerInfo())
	assert.Equal(t, "test-server", ts.ServerInfo().Name)
}

func TestToolTimeout(t *testing.T) {
	ts, _ := connectTestToolset(t)

	tl := newMCPTool("test", &mcp.Tool{
		Name:        "greet",
		InputSchema: map[string]any{"type": "object"},
	}, ts.session, time.Nanosecond)
	time.Sleep(time.Millisecond)

	_, err := tl.Process(context.Background(), map[string]any{"name": "World"})
	require.Error(t, err)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPTransportAppliesHeaders(t *testing.T) {
	var got http.Header
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})

	client := &http.Client{Transport: base}
	tr, err := newTransport(&transportConfig{
		url:        "http://example.com/mcp",
		headers:    map[string]string{"Authorization": "Bearer token"},
		httpClient: client,
	})
	require.NoError(t, err)

	st, ok := tr.(*mcp.StreamableClientTransport)
	require.True(t, ok)

	req, err := http.NewRequest(http.MethodPost, "http://example.com/mcp", nil)
	require.NoError(t, err)
	_, err = st.HTTPClient.Transport.RoundTrip(req)
	require.NoError(t, err)

	assert.Equal(t, "Bearer token", got.Get("Authorization"))
	_, wrapped := client.Transport.(*headerRoundTripper)
	assert.False(t, wrapped)
}

func TestHTTPTransportOptions(t *testing.T) {
	tr, err := newTransport(&transportConfig{
		url:                  "http://example.com/mcp",
		maxEventSize:         4096,
		maxRetries:           7,
		disableStandaloneSSE: true,
	})
	require.NoError(t, err)

	st, ok := tr.(*mcp.StreamableClientTransport)
	require.True(t, ok)
	assert.Equal(t, 4096, st.MaxEventSize)
	assert.Equal(t, 7, st.MaxRetries)
	assert.True(t, st.DisableStandaloneSSE)
	assert.Zero(t, st.HTTPClient.Timeout)
}

func TestHTTPTransportDefaultEventSize(t *testing.T) {
	tr, err := newTransport(&transportConfig{url: "http://example.com/mcp"})
	require.NoError(t, err)

	st, ok := tr.(*mcp.StreamableClientTransport)
	require.True(t, ok)
	assert.Equal(t, mcp.DefaultMaxEventSize, st.MaxEventSize)
}

func TestConvertInputSchema(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		expect *schema.Schema
	}{
		{
			name:   "nil defaults to empty object",
			input:  nil,
			expect: schema.Object(nil),
		},
		{
			name: "map with type and properties",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "the name",
					},
				},
				"required": []any{"name"},
			},
			expect: &schema.Schema{
				Type:       "object",
				Properties: map[string]*schema.Schema{"name": {Type: "string", Description: "the name"}},
				Required:   []string{"name"},
			},
		},
		{
			name:  "string type",
			input: map[string]any{"type": "string", "description": "a value"},
			expect: &schema.Schema{
				Type:        "string",
				Description: "a value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertInputSchema(tt.input)
			assert.Equal(t, tt.expect.Type, result.Type)
			if tt.expect.Properties != nil {
				for k, v := range tt.expect.Properties {
					assert.Equal(t, v.Type, result.Properties[k].Type)
					assert.Equal(t, v.Description, result.Properties[k].Description)
				}
			}
			if tt.expect.Required != nil {
				assert.Equal(t, tt.expect.Required, result.Required)
			}
		})
	}
}

func TestIsMethodNotFound(t *testing.T) {
	assert.True(t, isMethodNotFound(&jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound}))
	assert.False(t, isMethodNotFound(&jsonrpc.Error{Code: jsonrpc.CodeInternalError}))
	assert.False(t, isMethodNotFound(errors.New("boom")))
}

func TestSanitizeName(t *testing.T) {
	assert.Equal(t, "hello_world", sanitizeName("hello world"))
	assert.Equal(t, "my-server", sanitizeName("my-server"))
	assert.Equal(t, "foo_bar", sanitizeName("foo.bar"))
	assert.Equal(t, "test123", sanitizeName("test123"))
}
