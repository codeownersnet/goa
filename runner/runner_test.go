package runner

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

func TestRunnerFeedsAppendedToolResultsBackToModel(t *testing.T) {
	ctx := context.Background()
	fm := &statefulToolModel{t: t}
	testTool, err := functiontool.New(functiontool.Config{
		Name:        "record",
		Description: "Records a value.",
	}, func(context.Context, recordArgs) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
	require.NoError(t, err)

	a, err := llmagent.New(llmagent.Config{
		Name:  "test-agent",
		Model: fm,
		Tools: []tool.Tool{testTool},
	})
	require.NoError(t, err)

	r, err := New(Config{
		AppName:           "test-app",
		Agent:             a,
		SessionService:    session.InMemoryService(),
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memory.InMemoryService(),
		AutoCreateSession: true,
	})
	require.NoError(t, err)

	var events []*session.Event
	for ev, runErr := range r.Run(ctx, "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		require.NoError(t, runErr)
		events = append(events, ev)
	}

	require.Len(t, events, 3)
	assert.Equal(t, 2, fm.calls)
	assert.Equal(t, "done", events[len(events)-1].Text())
}

func TestRunnerPassesAgentInstructionToModel(t *testing.T) {
	ctx := context.Background()
	fm := &instructionModel{t: t, want: "agent instruction"}

	a, err := llmagent.New(llmagent.Config{
		Name:        "test-agent",
		Model:       fm,
		Instruction: "agent instruction",
	})
	require.NoError(t, err)

	r, err := New(Config{
		AppName:           "test-app",
		Agent:             a,
		SessionService:    session.InMemoryService(),
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memory.InMemoryService(),
		AutoCreateSession: true,
	})
	require.NoError(t, err)

	var events []*session.Event
	for ev, runErr := range r.Run(ctx, "user", "session", content.NewTextContent("do it", content.RoleUser), agent.RunConfig{}) {
		require.NoError(t, runErr)
		events = append(events, ev)
	}

	require.Len(t, events, 1)
	assert.Equal(t, "done", events[0].Text())
}

type recordArgs struct {
	Value string `json:"value"`
}

type statefulToolModel struct {
	t     *testing.T
	calls int
}

type instructionModel struct {
	t    *testing.T
	want string
}

func (m *instructionModel) Name() string { return "fake" }

func (m *instructionModel) Capabilities() model.ModelCapabilities { return model.ModelCapabilities{} }

func (m *instructionModel) GenerateContent(_ context.Context, req *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		require.NotNil(m.t, req.Config)
		require.Equal(m.t, m.want, req.Config.SystemInstruction)
		yield(&model.ModelResponse{Content: content.NewTextContent("done", content.RoleModel)}, nil)
	}
}

func (m *statefulToolModel) Name() string { return "fake" }

func (m *statefulToolModel) Capabilities() model.ModelCapabilities { return model.ModelCapabilities{} }

func (m *statefulToolModel) GenerateContent(_ context.Context, req *model.ModelRequest, _ bool) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		m.calls++
		switch m.calls {
		case 1:
			require.Len(m.t, req.Contents, 1)
			require.Equal(m.t, content.RoleUser, req.Contents[0].Role)
			yield(&model.ModelResponse{
				Content: content.NewContent(content.RoleModel, content.NewFunctionCallPart("call-1", "record", map[string]any{"value": "x"})),
			}, nil)
		case 2:
			require.Len(m.t, req.Contents, 3)
			require.Equal(m.t, content.RoleUser, req.Contents[0].Role)
			require.Equal(m.t, content.RoleModel, req.Contents[1].Role)
			require.Equal(m.t, content.RoleTool, req.Contents[2].Role)
			require.NotNil(m.t, req.Contents[2].Parts[0].FunctionResponse)
			yield(&model.ModelResponse{Content: content.NewTextContent("done", content.RoleModel)}, nil)
		default:
			yield(nil, fmt.Errorf("unexpected model call %d", m.calls))
		}
	}
}
