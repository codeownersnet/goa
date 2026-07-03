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
