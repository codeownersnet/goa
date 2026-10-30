package flow

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/testutil"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

func TestRequireToolUseRejectsTextOnlyResponse(t *testing.T) {
	ctx := context.Background()
	m := &textOnlyModel{}

	testTool, err := functiontool.New(functiontool.Config{
		Name:        "helper",
		Description: "A helper tool.",
	}, func(context.Context, struct{}) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
	require.NoError(t, err)

	a := agent.New(
		agent.WithName("test"),
		agent.WithDescription("test"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			f := &Flow{
				Model:          m,
				Tools:          []tool.Tool{testTool},
				Instruction:    "Do work",
				RequireToolUse: true,
			}
			return f.Run(ctx)
		}),
	)

	r, err := runner.New(runner.Config{
		AppName:           "test-app",
		Agent:             a,
		SessionService:    session.InMemoryService(),
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memory.InMemoryService(),
		AutoCreateSession: true,
	})
	require.NoError(t, err)

	var gotErr error
	for _, runErr := range r.Run(ctx, "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		if runErr != nil {
			gotErr = runErr
		}
	}
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "text-only response")
}

func TestRequireToolUseAcceptsAfterToolUse(t *testing.T) {
	ctx := context.Background()
	m := &toolThenTextModel{t: t}

	testTool, err := functiontool.New(functiontool.Config{
		Name:        "record",
		Description: "Records a value.",
	}, func(context.Context, struct{}) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
	require.NoError(t, err)

	a := agent.New(
		agent.WithName("test"),
		agent.WithDescription("test"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			f := &Flow{
				Model:          m,
				Tools:          []tool.Tool{testTool},
				Instruction:    "Do work",
				RequireToolUse: true,
			}
			return f.Run(ctx)
		}),
	)

	r, err := runner.New(runner.Config{
		AppName:           "test-app",
		Agent:             a,
		SessionService:    session.InMemoryService(),
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memory.InMemoryService(),
		AutoCreateSession: true,
	})
	require.NoError(t, err)

	var events []*session.Event
	var gotErr error
	for ev, runErr := range r.Run(ctx, "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		if runErr != nil {
			gotErr = runErr
			break
		}
		if ev != nil {
			events = append(events, ev)
		}
	}
	assert.NoError(t, gotErr)
	assert.True(t, len(events) > 0)
}

type textOnlyModel struct{}

func (m *textOnlyModel) Name() string { return "fake" }
func (m *textOnlyModel) Capabilities() model.ModelCapabilities {
	return model.ModelCapabilities{}
}
func (m *textOnlyModel) GenerateContent(_ context.Context, _ *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		yield(&model.ModelResponse{
			Content:      content.NewTextContent("I need more information to proceed.", content.RoleModel),
			FinishReason: model.FinishReasonStop,
		}, nil)
	}
}

type toolThenTextModel struct {
	t     *testing.T
	calls int
}

func (m *toolThenTextModel) Name() string { return "fake" }
func (m *toolThenTextModel) Capabilities() model.ModelCapabilities {
	return model.ModelCapabilities{}
}
func (m *toolThenTextModel) GenerateContent(_ context.Context, _ *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		m.calls++
		switch m.calls {
		case 1:
			yield(&model.ModelResponse{
				Content: content.NewContent(content.RoleModel,
					content.NewFunctionCallPart("call-1", "record", map[string]any{}),
				),
			}, nil)
		case 2:
			yield(&model.ModelResponse{
				Content:      content.NewTextContent("Done!", content.RoleModel),
				FinishReason: model.FinishReasonStop,
			}, nil)
		default:
			yield(nil, fmt.Errorf("unexpected call %d", m.calls))
		}
	}
}

type capturingArgsModel struct {
	calls int
}

func (m *capturingArgsModel) Name() string { return "fake-calls" }
func (m *capturingArgsModel) Capabilities() model.ModelCapabilities {
	return model.ModelCapabilities{}
}
func (m *capturingArgsModel) GenerateContent(_ context.Context, _ *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		m.calls++
		if m.calls == 1 {
			yield(&model.ModelResponse{
				Content: content.NewContent(content.RoleModel,
					content.NewFunctionCallPart("call-1", "echo", map[string]any{"value": "original"}),
				),
			}, nil)
			return
		}
		yield(&model.ModelResponse{
			Content:      content.NewTextContent("Done!", content.RoleModel),
			FinishReason: model.FinishReasonStop,
		}, nil)
	}
}

type echoArgs struct {
	Value string `json:"value"`
}

func runFlowWithCallbacks(t *testing.T, m model.Model, cfg agent.RunConfig, before []BeforeToolCallback, gotArgs *map[string]any) {
	t.Helper()

	echoTool, err := functiontool.New(functiontool.Config{
		Name:        "echo",
		Description: "Echoes a value.",
	}, func(_ context.Context, args echoArgs) (map[string]any, error) {
		*gotArgs = map[string]any{"value": args.Value}
		return map[string]any{"value": args.Value}, nil
	})
	require.NoError(t, err)

	a := agent.New(
		agent.WithName("test"),
		agent.WithDescription("test"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			f := &Flow{
				Model:               m,
				Tools:               []tool.Tool{echoTool},
				Instruction:         "Do work",
				BeforeToolCallbacks: before,
			}
			return f.Run(ctx)
		}),
	)

	r, err := runner.New(runner.Config{
		AppName:           "test-app",
		Agent:             a,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	require.NoError(t, err)

	for _, runErr := range r.Run(context.Background(), "user", "session", content.NewTextContent("do it", content.RoleUser), cfg) {
		require.NoError(t, runErr)
	}
}

func TestBeforeToolCallbackPassThroughLeavesArgsIntact(t *testing.T) {
	var got map[string]any
	runFlowWithCallbacks(t, &capturingArgsModel{}, agent.RunConfig{}, []BeforeToolCallback{
		func(_ tool.Context, _ tool.Tool, _ map[string]any) (map[string]any, error) {
			return nil, nil
		},
	}, &got)
	assert.Equal(t, map[string]any{"value": "original"}, got)
}

func TestBeforeToolCallbackRewritesArgs(t *testing.T) {
	var got map[string]any
	runFlowWithCallbacks(t, &capturingArgsModel{}, agent.RunConfig{}, []BeforeToolCallback{
		func(_ tool.Context, _ tool.Tool, _ map[string]any) (map[string]any, error) {
			return map[string]any{"value": "rewritten"}, nil
		},
	}, &got)
	assert.Equal(t, map[string]any{"value": "rewritten"}, got)
}

func TestBeforeToolCallbackErrorAbortsToolCall(t *testing.T) {
	var got map[string]any
	runFlowWithCallbacks(t, &capturingArgsModel{}, agent.RunConfig{}, []BeforeToolCallback{
		func(_ tool.Context, _ tool.Tool, _ map[string]any) (map[string]any, error) {
			return nil, fmt.Errorf("callback rejected call")
		},
	}, &got)
	assert.Nil(t, got)
}

func TestStreamingFlagFromRunConfig(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cfg    agent.RunConfig
		stream bool
	}{
		{name: "SSE mode streams", cfg: agent.RunConfig{StreamingMode: agent.StreamingModeSSE}, stream: true},
		{name: "None mode does not stream", cfg: agent.RunConfig{StreamingMode: agent.StreamingModeNone}, stream: false},
		{name: "default does not stream", cfg: agent.RunConfig{}, stream: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &testutil.ScriptedModel{
				Script: func(_ int, _ *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
					return func(yield func(*model.ModelResponse, error) bool) {
						yield(&model.ModelResponse{
							Content:      content.NewTextContent("Done!", content.RoleModel),
							FinishReason: model.FinishReasonStop,
						}, nil)
					}
				},
			}
			var ignored map[string]any
			runFlowWithCallbacks(t, m, tc.cfg, nil, &ignored)
			require.NotEmpty(t, m.Streams())
			for _, s := range m.Streams() {
				assert.Equal(t, tc.stream, s)
			}
		})
	}
}

func TestUnknownToolReturnsErrorResponseWithAvailableTools(t *testing.T) {
	ctx := context.Background()
	m := &unknownToolModel{}

	echoTool, err := functiontool.New(functiontool.Config{
		Name:        "echo",
		Description: "Echoes a value.",
	}, func(_ context.Context, args echoArgs) (map[string]any, error) {
		return map[string]any{"value": args.Value}, nil
	})
	require.NoError(t, err)

	a := agent.New(
		agent.WithName("test"),
		agent.WithDescription("test"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			f := &Flow{Model: m, Tools: []tool.Tool{echoTool}, Instruction: "Do work"}
			return f.Run(ctx)
		}),
	)

	r, err := runner.New(runner.Config{
		AppName:           "test-app",
		Agent:             a,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	require.NoError(t, err)

	var events []*session.Event
	for ev, runErr := range r.Run(ctx, "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		require.NoError(t, runErr)
		if ev != nil {
			events = append(events, ev)
		}
	}

	var foundErr bool
	var namesOK bool
	for _, ev := range events {
		if ev.ModelResponse == nil {
			continue
		}
		for _, part := range ev.ModelResponse.Parts {
			if part.FunctionResponse != nil && part.FunctionResponse.IsError {
				foundErr = true
				if names, ok := part.FunctionResponse.Response["available_tools"].([]string); ok {
					namesOK = true
					assert.NotEmpty(t, names)
				}
			}
		}
	}
	require.True(t, foundErr, "expected an error function response for the unknown tool")
	assert.True(t, namesOK, "expected available_tools listing in the error response")
}

type unknownToolModel struct {
	calls int
}

func (m *unknownToolModel) Name() string { return "fake-unknown" }
func (m *unknownToolModel) Capabilities() model.ModelCapabilities {
	return model.ModelCapabilities{}
}
func (m *unknownToolModel) GenerateContent(_ context.Context, _ *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		m.calls++
		if m.calls == 1 {
			yield(&model.ModelResponse{
				Content: content.NewContent(content.RoleModel,
					content.NewFunctionCallPart("call-1", "does_not_exist", map[string]any{}),
				),
			}, nil)
			return
		}
		yield(&model.ModelResponse{
			Content:      content.NewTextContent("Done!", content.RoleModel),
			FinishReason: model.FinishReasonStop,
		}, nil)
	}
}
