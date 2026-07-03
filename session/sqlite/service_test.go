package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/session"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(context.Background(), Config{Path: ":memory:"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestCreateAndGet(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   "test-app",
		UserID:    "user1",
		SessionID: "sess1",
		State:     map[string]any{"key1": "val1"},
	})
	require.NoError(t, err)
	assert.Equal(t, "sess1", resp.Session.ID())
	assert.Equal(t, "test-app", resp.Session.AppName())
	assert.Equal(t, "user1", resp.Session.UserID())

	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName:   "test-app",
		UserID:    "user1",
		SessionID: "sess1",
	})
	require.NoError(t, err)
	assert.Equal(t, "sess1", getResp.Session.ID())

	val, err := getResp.Session.State().Get("key1")
	require.NoError(t, err)
	assert.Equal(t, "val1", val)
}

func TestCreateDuplicateSession(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	_, err = svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	assert.NoError(t, err)
}

func TestGetNotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "u1", SessionID: "missing",
	})
	assert.Error(t, err)
}

func TestListSessions(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName: "app", UserID: "u1", SessionID: fmt.Sprintf("s%d", i),
		})
		require.NoError(t, err)
	}
	_, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "other-app", UserID: "u1", SessionID: "other",
	})
	require.NoError(t, err)

	resp, err := svc.List(ctx, &session.ListRequest{AppName: "app"})
	require.NoError(t, err)
	assert.Len(t, resp.Sessions, 3)

	resp, err = svc.List(ctx, &session.ListRequest{AppName: "app", UserID: "u1"})
	require.NoError(t, err)
	assert.Len(t, resp.Sessions, 3)

	resp, err = svc.List(ctx, &session.ListRequest{AppName: "other-app"})
	require.NoError(t, err)
	assert.Len(t, resp.Sessions, 1)
}

func TestDeleteSession(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	err = svc.Delete(ctx, &session.DeleteRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	_, err = svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	assert.Error(t, err)
}

func TestAppendEvent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
		State: map[string]any{},
	})
	require.NoError(t, err)

	event := session.NewEvent("inv-1")
	event.Author = "agent"
	event.Actions = session.EventActions{StateDelta: map[string]any{"count": 1}}

	err = svc.AppendEvent(ctx, createResp.Session, event)
	require.NoError(t, err)

	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	val, err := getResp.Session.State().Get("count")
	require.NoError(t, err)
	assert.Equal(t, 1.0, val)

	assert.Equal(t, 1, getResp.Session.Events().Len())
}

func TestAppendPartialEvent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	event := session.NewEvent("inv-1")
	event.Partial = true

	err = svc.AppendEvent(ctx, createResp.Session, event)
	require.NoError(t, err)

	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, getResp.Session.Events().Len())
}

func TestStateDeltaMerge(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
		State: map[string]any{"existing": "value"},
	})
	require.NoError(t, err)

	event1 := session.NewEvent("inv-1")
	event1.Actions = session.EventActions{StateDelta: map[string]any{"new_key": "new_val"}}
	err = svc.AppendEvent(ctx, createResp.Session, event1)
	require.NoError(t, err)

	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	existing, err := getResp.Session.State().Get("existing")
	require.NoError(t, err)
	assert.Equal(t, "value", existing)

	newVal, err := getResp.Session.State().Get("new_key")
	require.NoError(t, err)
	assert.Equal(t, "new_val", newVal)
}

func TestStateDeltaMergeWithNilInitialState(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	event := session.NewEvent("inv-1")
	event.Actions = session.EventActions{StateDelta: map[string]any{"planned": true}}
	err = svc.AppendEvent(ctx, createResp.Session, event)
	require.NoError(t, err)

	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	planned, err := getResp.Session.State().Get("planned")
	require.NoError(t, err)
	assert.Equal(t, true, planned)
}

func TestGetWithNumRecentEvents(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	for i := range 5 {
		ev := session.NewEvent("inv-1")
		ev.Author = fmt.Sprintf("agent-%d", i)
		err = svc.AppendEvent(ctx, createResp.Session, ev)
		require.NoError(t, err)
	}

	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName:         "app",
		UserID:          "u1",
		SessionID:       "s1",
		NumRecentEvents: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, getResp.Session.Events().Len())
}

func TestSessionStateGetMissing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
		State: map[string]any{},
	})
	require.NoError(t, err)

	_, err = createResp.Session.State().Get("nonexistent")
	assert.ErrorIs(t, err, session.ErrStateKeyNotExist)
}

func TestEventsIteration(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	for i := range 3 {
		ev := session.NewEvent("inv-1")
		ev.Author = fmt.Sprintf("agent-%d", i)
		err = svc.AppendEvent(ctx, createResp.Session, ev)
		require.NoError(t, err)
	}

	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	var authors []string
	for ev := range getResp.Session.Events().All() {
		authors = append(authors, ev.Author)
	}
	assert.Equal(t, []string{"agent-0", "agent-1", "agent-2"}, authors)
}

func TestAutoSessionID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp1, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp1.Session.ID())

	resp2, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "u1",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp2.Session.ID())
	assert.NotEqual(t, resp1.Session.ID(), resp2.Session.ID())
}
