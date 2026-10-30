package flow_test

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/flow"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/testutil"
)

func TestHostManagedInvocationWithoutRunner(t *testing.T) {
	ctx := context.Background()
	m := &testutil.ScriptedModel{Script: func(int, *model.ModelRequest, bool) iter.Seq2[*model.ModelResponse, error] {
		return func(yield func(*model.ModelResponse, error) bool) {
			yield(&model.ModelResponse{
				Content:      content.NewTextContent("done", content.RoleModel),
				FinishReason: model.FinishReasonStop,
				TurnComplete: true,
			}, nil)
		}
	}}

	a := agent.New(
		agent.WithName("host-agent"),
		agent.WithDescription("host-managed"),
		agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return (&flow.Flow{Model: m, Instruction: "Do work"}).Run(ctx)
		}),
	)

	svc := session.InMemoryService()
	createResp, err := svc.Create(ctx, &session.CreateRequest{AppName: "app", UserID: "user", SessionID: "session"})
	require.NoError(t, err)

	runConfig := agent.RunConfig{}
	invCtx := agent.NewInvocationContext(
		ctx, a, artifact.InMemoryService(), memory.InMemoryService(), createResp.Session,
		"inv-1", "host-agent", content.NewTextContent("do it", content.RoleUser), &runConfig,
	)

	var events []*session.Event
	for event, runErr := range a.Run(invCtx) {
		require.NoError(t, runErr)
		events = append(events, event)
		if !event.Partial {
			require.NoError(t, svc.AppendEvent(ctx, createResp.Session, event))
		}
	}

	require.Len(t, events, 1)
	assert.Equal(t, "done", events[0].Text())
	getResp, err := svc.Get(ctx, &session.GetRequest{AppName: "app", UserID: "user", SessionID: "session"})
	require.NoError(t, err)
	require.Equal(t, 1, getResp.Session.Events().Len())
	assert.Equal(t, "done", getResp.Session.Events().At(0).Text())
}
