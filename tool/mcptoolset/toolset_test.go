package mcptoolset

import (
	"context"
	"strings"
	"testing"

	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) *mcp.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)

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

	require.NoError(t, ts.discoverTools(context.Background(), 60000))
	require.NoError(t, ts.discoverResources(context.Background()))
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

func TestSanitizeName(t *testing.T) {
	assert.Equal(t, "hello_world", sanitizeName("hello world"))
	assert.Equal(t, "my-server", sanitizeName("my-server"))
	assert.Equal(t, "foo_bar", sanitizeName("foo.bar"))
	assert.Equal(t, "test123", sanitizeName("test123"))
}
