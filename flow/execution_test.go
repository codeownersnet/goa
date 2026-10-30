package flow

import (
	"context"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/testutil"
	"github.com/codeownersnet/goa/tool"
)

type execTool struct {
	name string
	run  func(context.Context, map[string]any) (map[string]any, error)
}

func (t *execTool) Name() string        { return t.name }
func (t *execTool) Description() string { return t.name }
func (t *execTool) Process(ctx context.Context, args map[string]any) (map[string]any, error) {
	return t.run(ctx, args)
}

func runExecution(t *testing.T, m model.Model, tools []tool.Tool, opts ExecutionOptions) ([]*session.Event, error) {
	t.Helper()
	a := agent.New(
		agent.WithName("test"),
		agent.WithDescription("test"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			f := &Flow{Model: m, Tools: tools, Instruction: "Do work", ExecutionOptions: opts}
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
	for ev, runErr := range r.Run(context.Background(), "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		if runErr != nil {
			return events, runErr
		}
		events = append(events, ev)
	}
	return events, nil
}

func finalResponse(text string) *model.ModelResponse {
	return &model.ModelResponse{Content: content.NewTextContent(text, content.RoleModel), FinishReason: model.FinishReasonStop}
}

func toolCallResponse(calls ...*content.FunctionCall) *model.ModelResponse {
	parts := make([]content.Part, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, content.NewFunctionCallPart(call.ID, call.Name, call.Args))
	}
	return &model.ModelResponse{
		Content:      content.NewContent(content.RoleModel, parts...),
		FinishReason: model.FinishReasonToolCall,
	}
}

func scripted(responses ...func(int) *model.ModelResponse) *testutil.ScriptedModel {
	return &testutil.ScriptedModel{Script: func(call int, _ *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
		return func(yield func(*model.ModelResponse, error) bool) {
			if call-1 < len(responses) {
				yield(responses[call-1](call), nil)
				return
			}
			yield(finalResponse("done"), nil)
		}
	}}
}

func TestSequentialToolSchedulingPreservesOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	mkTool := func(name string) tool.Tool {
		return &execTool{name: name, run: func(context.Context, map[string]any) (map[string]any, error) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return map[string]any{"ok": true}, nil
		}}
	}
	m := scripted(func(int) *model.ModelResponse {
		return toolCallResponse(
			&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}},
			&content.FunctionCall{ID: "2", Name: "b", Args: map[string]any{}},
		)
	})

	_, err := runExecution(t, m, []tool.Tool{mkTool("a"), mkTool("b")}, ExecutionOptions{ToolScheduling: ToolSchedulingSequential})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, order)
}

func TestParallelToolSchedulingRespectsCap(t *testing.T) {
	var mu sync.Mutex
	current, peak := 0, 0
	mkTool := func(name string) tool.Tool {
		return &execTool{name: name, run: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			mu.Lock()
			current++
			if current > peak {
				peak = current
			}
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			current--
			mu.Unlock()
			return map[string]any{"ok": true}, nil
		}}
	}
	m := scripted(func(int) *model.ModelResponse {
		return toolCallResponse(
			&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}},
			&content.FunctionCall{ID: "2", Name: "b", Args: map[string]any{}},
			&content.FunctionCall{ID: "3", Name: "c", Args: map[string]any{}},
		)
	})

	_, err := runExecution(t, m, []tool.Tool{mkTool("a"), mkTool("b"), mkTool("c")}, ExecutionOptions{MaxParallelTools: 2})
	require.NoError(t, err)
	assert.LessOrEqual(t, peak, 2)
}

func TestUncertainToolOutcomeStopsSequentialRun(t *testing.T) {
	bRan := false
	a := &execTool{name: "a", run: func(context.Context, map[string]any) (map[string]any, error) {
		return nil, tool.NewUncertainError(assert.AnError)
	}}
	b := &execTool{name: "b", run: func(context.Context, map[string]any) (map[string]any, error) {
		bRan = true
		return map[string]any{"ok": true}, nil
	}}
	m := scripted(func(int) *model.ModelResponse {
		return toolCallResponse(
			&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}},
			&content.FunctionCall{ID: "2", Name: "b", Args: map[string]any{}},
		)
	})

	_, err := runExecution(t, m, []tool.Tool{a, b}, ExecutionOptions{ToolScheduling: ToolSchedulingSequential})
	require.Error(t, err)
	assert.True(t, tool.IsTerminal(err))
	assert.False(t, bRan)
	assert.Equal(t, 1, m.Calls())
}

func TestGuardDenialCannotBeSwallowedByErrorCallback(t *testing.T) {
	callbackCalled := false
	executed := false
	deny := &execTool{name: "a", run: func(context.Context, map[string]any) (map[string]any, error) {
		executed = true
		return map[string]any{"ok": true}, nil
	}}
	m := scripted(func(int) *model.ModelResponse {
		return toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}})
	})
	a := agent.New(
		agent.WithName("test"),
		agent.WithDescription("test"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			f := &Flow{
				Model: m,
				Tools: []tool.Tool{deny},
				ExecutionOptions: ExecutionOptions{
					BeforeToolGuards: []ToolGuard{func(tool.Context, tool.Tool, map[string]any) error {
						return assert.AnError
					}},
				},
				OnToolErrorCallbacks: []OnToolErrorCallback{func(tool.Context, tool.Tool, map[string]any, error) (map[string]any, error) {
					callbackCalled = true
					return map[string]any{"ok": true}, nil
				}},
			}
			return f.Run(ctx)
		}),
	)
	r, err := runner.New(runner.Config{AppName: "test-app", Agent: a, SessionService: session.InMemoryService(), AutoCreateSession: true})
	require.NoError(t, err)
	for _, runErr := range r.Run(context.Background(), "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		if runErr != nil {
			require.ErrorIs(t, runErr, assert.AnError)
			break
		}
	}
	assert.False(t, callbackCalled)
	assert.False(t, executed)
}

func TestBeforeModelGuardBlocksDispatch(t *testing.T) {
	m := scripted(func(int) *model.ModelResponse { return finalResponse("done") })
	_, err := runExecution(t, m, nil, ExecutionOptions{
		BeforeModelGuards: []ModelGuard{func(agent.InvocationContext, *model.ModelRequest) error { return assert.AnError }},
	})
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 0, m.Calls())
}

func TestCompletionGuardRejectsFinalResponse(t *testing.T) {
	m := scripted(func(int) *model.ModelResponse { return finalResponse("done") })
	_, err := runExecution(t, m, nil, ExecutionOptions{
		BeforeCompletionGuards: []CompletionGuard{func(agent.InvocationContext, *session.Event) error { return assert.AnError }},
	})
	require.ErrorIs(t, err, assert.AnError)
}

func TestRunLimits(t *testing.T) {
	t.Run("model turns", func(t *testing.T) {
		ran := false
		it := &execTool{name: "a", run: func(context.Context, map[string]any) (map[string]any, error) {
			ran = true
			return map[string]any{"ok": true}, nil
		}}
		m := scripted(func(int) *model.ModelResponse {
			return toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}})
		})
		_, err := runExecution(t, m, []tool.Tool{it}, ExecutionOptions{Limits: RunLimits{MaxModelTurns: 1}})
		require.Error(t, err)
		assert.True(t, ran)
		assert.Equal(t, 1, m.Calls())
	})

	t.Run("tool calls per turn", func(t *testing.T) {
		m := scripted(func(int) *model.ModelResponse {
			return toolCallResponse(
				&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}},
				&content.FunctionCall{ID: "2", Name: "b", Args: map[string]any{}},
			)
		})
		_, err := runExecution(t, m, nil, ExecutionOptions{Limits: RunLimits{MaxToolCallsPerTurn: 1}})
		require.Error(t, err)
	})

	t.Run("resumed tool call budget", func(t *testing.T) {
		m := scripted(func(int) *model.ModelResponse {
			return toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}})
		})
		budget := &RunBudget{ToolCalls: 1}
		_, err := runExecution(t, m, nil, ExecutionOptions{Limits: RunLimits{MaxToolCalls: 1, Budget: budget}})
		require.Error(t, err)
	})

	t.Run("prompt bytes", func(t *testing.T) {
		m := scripted(func(int) *model.ModelResponse { return finalResponse("done") })
		_, err := runExecution(t, m, nil, ExecutionOptions{Limits: RunLimits{MaxPromptBytes: 1}})
		require.Error(t, err)
		assert.Equal(t, 0, m.Calls())
	})

	t.Run("output bytes", func(t *testing.T) {
		m := scripted(func(int) *model.ModelResponse { return finalResponse("hello") })
		_, err := runExecution(t, m, nil, ExecutionOptions{Limits: RunLimits{MaxOutputBytes: 1}})
		require.Error(t, err)
	})

	t.Run("tool duration", func(t *testing.T) {
		slow := &execTool{name: "a", run: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return map[string]any{"ok": true}, nil
			}
		}}
		m := scripted(func(int) *model.ModelResponse {
			return toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}})
		})
		_, err := runExecution(t, m, []tool.Tool{slow}, ExecutionOptions{Limits: RunLimits{MaxToolDuration: 5 * time.Millisecond}})
		require.Error(t, err)
		assert.Equal(t, 1, m.Calls())
	})

	t.Run("tool result bytes", func(t *testing.T) {
		big := &execTool{name: "a", run: func(context.Context, map[string]any) (map[string]any, error) {
			return map[string]any{"value": strings.Repeat("x", 100)}, nil
		}}
		m := scripted(func(int) *model.ModelResponse {
			return toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}})
		})
		_, err := runExecution(t, m, []tool.Tool{big}, ExecutionOptions{Limits: RunLimits{MaxToolResultBytes: 10}})
		require.Error(t, err)
		assert.True(t, tool.IsTerminal(err))
		assert.Equal(t, 1, m.Calls())
	})
}

func TestSlidingWindowContextPolicy(t *testing.T) {
	req := &model.ModelRequest{Contents: []*content.Content{
		content.NewTextContent(strings.Repeat("a", 100), content.RoleUser),
		content.NewTextContent(strings.Repeat("b", 100), content.RoleModel),
		content.NewTextContent(strings.Repeat("c", 100), content.RoleUser),
		content.NewTextContent(strings.Repeat("d", 100), content.RoleUser),
	}}
	last := req.Contents[len(req.Contents)-1]
	policy := SlidingWindowContextPolicy(250)
	require.NoError(t, policy.Apply(nil, req))
	size, err := requestBytes(req)
	require.NoError(t, err)
	assert.LessOrEqual(t, size, 250)
	assert.Same(t, last, req.Contents[len(req.Contents)-1])
}

func TestOutputValidation(t *testing.T) {
	validator := OutputValidatorFunc(func(_ agent.InvocationContext, event *session.Event) error {
		if event.Text() == "bad" {
			return assert.AnError
		}
		return nil
	})

	t.Run("failure stops run", func(t *testing.T) {
		m := scripted(func(int) *model.ModelResponse { return finalResponse("bad") })
		_, err := runExecution(t, m, nil, ExecutionOptions{OutputValidator: validator})
		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, m.Calls())
	})

	t.Run("bounded repair loop", func(t *testing.T) {
		m := scripted(
			func(int) *model.ModelResponse { return finalResponse("bad") },
			func(int) *model.ModelResponse { return finalResponse("good") },
		)
		events, err := runExecution(t, m, nil, ExecutionOptions{OutputValidator: validator, MaxValidationRepairs: 1})
		require.NoError(t, err)
		assert.Equal(t, 2, m.Calls())
		foundRepair := false
		for _, event := range events {
			if event.Author == "system" && strings.Contains(event.Text(), "failed validation") {
				foundRepair = true
			}
		}
		assert.True(t, foundRepair)
	})
}

type capabilityModel struct {
	*testutil.ScriptedModel
	caps model.ModelCapabilities
}

func (m *capabilityModel) Capabilities() model.ModelCapabilities { return m.caps }

func TestRequiredCapabilitiesFailVisibly(t *testing.T) {
	m := &capabilityModel{
		ScriptedModel: scripted(func(int) *model.ModelResponse { return finalResponse("done") }),
		caps:          model.ModelCapabilities{},
	}
	_, err := runExecution(t, m, nil, ExecutionOptions{RequiredCapabilities: RequiredCapabilities{ToolCall: true}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool_call")
	assert.Equal(t, 0, m.Calls())
}

func TestTerminalToolOutcomeBypassesAfterToolCallback(t *testing.T) {
	callbackCalled := false
	uncertain := &execTool{name: "a", run: func(context.Context, map[string]any) (map[string]any, error) {
		return nil, tool.NewUncertainError(assert.AnError)
	}}
	m := scripted(func(int) *model.ModelResponse {
		return toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}})
	})
	a := agent.New(
		agent.WithName("test"),
		agent.WithDescription("test"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			f := &Flow{
				Model: m,
				Tools: []tool.Tool{uncertain},
				AfterToolCallbacks: []AfterToolCallback{func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error) {
					callbackCalled = true
					return map[string]any{"ok": true}, nil
				}},
			}
			return f.Run(ctx)
		}),
	)
	r, err := runner.New(runner.Config{AppName: "test-app", Agent: a, SessionService: session.InMemoryService(), AutoCreateSession: true})
	require.NoError(t, err)
	var runErr error
	for _, err := range r.Run(context.Background(), "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		if err != nil {
			runErr = err
			break
		}
	}
	require.Error(t, runErr)
	assert.True(t, tool.IsTerminal(runErr))
	assert.False(t, callbackCalled)
}

func TestRunDeadlineStopsDispatch(t *testing.T) {
	slow := &execTool{name: "a", run: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return map[string]any{"ok": true}, nil
		}
	}}
	m := scripted(func(int) *model.ModelResponse {
		return toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}})
	})
	start := time.Now()
	_, err := runExecution(t, m, []tool.Tool{slow}, ExecutionOptions{Limits: RunLimits{RunDeadline: 10 * time.Millisecond}})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 150*time.Millisecond)
	assert.Equal(t, 1, m.Calls())
}

func TestSlidingWindowContextPolicyIrreducibleOverflow(t *testing.T) {
	req := &model.ModelRequest{Contents: []*content.Content{
		content.NewTextContent(strings.Repeat("a", 200), content.RoleUser),
	}}
	err := SlidingWindowContextPolicy(10).Apply(nil, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires at least")
}

func TestOutputValidationRepairBudgetIsBounded(t *testing.T) {
	validator := OutputValidatorFunc(func(agent.InvocationContext, *session.Event) error {
		return assert.AnError
	})
	m := scripted(func(int) *model.ModelResponse { return finalResponse("bad") })
	_, err := runExecution(t, m, nil, ExecutionOptions{OutputValidator: validator, MaxValidationRepairs: 1})
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 2, m.Calls())
}

func TestRecoverableToolErrorReachesModel(t *testing.T) {
	failing := &execTool{name: "a", run: func(context.Context, map[string]any) (map[string]any, error) {
		return nil, assert.AnError
	}}
	var sawToolResponse bool
	m := &testutil.ScriptedModel{Script: func(call int, req *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
		return func(yield func(*model.ModelResponse, error) bool) {
			if call == 1 {
				yield(toolCallResponse(&content.FunctionCall{ID: "1", Name: "a", Args: map[string]any{}}), nil)
				return
			}
			for _, message := range req.Contents {
				if message.Role == content.RoleTool {
					sawToolResponse = true
				}
			}
			yield(finalResponse("done"), nil)
		}
	}}
	_, err := runExecution(t, m, []tool.Tool{failing}, ExecutionOptions{})
	require.NoError(t, err)
	assert.True(t, sawToolResponse)
	assert.Equal(t, 2, m.Calls())
}
