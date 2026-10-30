package session

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/content"
)

func createSession(t *testing.T, svc Service, appName, userID, sessionID string) Session {
	t.Helper()
	resp, err := svc.Create(context.Background(), &CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	require.NoError(t, err)
	return resp.Session
}

func newTextEvent(invocationID, text string) *Event {
	event := NewEvent(invocationID)
	event.Author = "model"
	event.ModelResponse = content.NewTextContent(text, content.RoleModel)
	return event
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	svc := InMemoryService()

	sess := createSession(t, svc, "app", "user", "s1")
	assert.Equal(t, "s1", sess.ID())
	assert.Equal(t, "app", sess.AppName())
	assert.Equal(t, "user", sess.UserID())

	_, err := svc.Create(ctx, &CreateRequest{AppName: "app", UserID: "user", SessionID: "s1"})
	assert.ErrorContains(t, err, "already exists")

	resp, err := svc.Create(ctx, &CreateRequest{AppName: "app", UserID: "user"})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Session.ID())
	getResp, err := svc.Get(ctx, &GetRequest{AppName: "app", UserID: "user", SessionID: resp.Session.ID()})
	require.NoError(t, err)
	assert.Equal(t, resp.Session.ID(), getResp.Session.ID())
}

func TestGetMissing(t *testing.T) {
	svc := InMemoryService()
	_, err := svc.Get(context.Background(), &GetRequest{AppName: "app", UserID: "user", SessionID: "nope"})
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestGetNumRecentEventsAndAfter(t *testing.T) {
	ctx := context.Background()
	svc := InMemoryService()
	sess := createSession(t, svc, "app", "user", "s1")

	base := time.Now()
	for i := 0; i < 5; i++ {
		event := newTextEvent("inv", fmt.Sprintf("event %d", i))
		event.Timestamp = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, svc.AppendEvent(ctx, sess, event))
	}

	resp, err := svc.Get(ctx, &GetRequest{AppName: "app", UserID: "user", SessionID: "s1"})
	require.NoError(t, err)
	assert.Equal(t, 5, resp.Session.Events().Len())

	resp, err = svc.Get(ctx, &GetRequest{AppName: "app", UserID: "user", SessionID: "s1", NumRecentEvents: 2})
	require.NoError(t, err)
	require.Equal(t, 2, resp.Session.Events().Len())
	assert.Equal(t, "event 3", resp.Session.Events().At(0).Text())
	assert.Equal(t, "event 4", resp.Session.Events().At(1).Text())

	resp, err = svc.Get(ctx, &GetRequest{AppName: "app", UserID: "user", SessionID: "s1", After: base.Add(3 * time.Minute)})
	require.NoError(t, err)
	require.Equal(t, 2, resp.Session.Events().Len())
	assert.Equal(t, "event 3", resp.Session.Events().At(0).Text())
	assert.Equal(t, "event 4", resp.Session.Events().At(1).Text())
}

func TestListFiltering(t *testing.T) {
	ctx := context.Background()
	svc := InMemoryService()

	createSession(t, svc, "app1", "user1", "s1")
	createSession(t, svc, "app1", "user2", "s2")
	createSession(t, svc, "app2", "user1", "s3")

	resp, err := svc.List(ctx, &ListRequest{AppName: "app1"})
	require.NoError(t, err)
	assert.Len(t, resp.Sessions, 2)

	resp, err = svc.List(ctx, &ListRequest{AppName: "app1", UserID: "user2"})
	require.NoError(t, err)
	require.Len(t, resp.Sessions, 1)
	assert.Equal(t, "s2", resp.Sessions[0].ID())

	resp, err = svc.List(ctx, &ListRequest{AppName: "app2", UserID: "user1"})
	require.NoError(t, err)
	require.Len(t, resp.Sessions, 1)
	assert.Equal(t, "s3", resp.Sessions[0].ID())

	_, err = svc.List(ctx, &ListRequest{})
	assert.Error(t, err)
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	svc := InMemoryService()
	createSession(t, svc, "app", "user", "s1")

	require.NoError(t, svc.Delete(ctx, &DeleteRequest{AppName: "app", UserID: "user", SessionID: "s1"}))

	_, err := svc.Get(ctx, &GetRequest{AppName: "app", UserID: "user", SessionID: "s1"})
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestAppendEvent(t *testing.T) {
	ctx := context.Background()
	svc := InMemoryService()
	sess := createSession(t, svc, "app", "user", "s1")

	event := newTextEvent("inv", "hello")
	event.Actions.StateDelta = map[string]any{"greeting": "hi", "count": 1}
	require.NoError(t, svc.AppendEvent(ctx, sess, event))

	partial := newTextEvent("inv", "partial chunk")
	partial.Partial = true
	partial.Actions.StateDelta = map[string]any{"partial-key": "nope"}
	require.NoError(t, svc.AppendEvent(ctx, sess, partial))

	resp, err := svc.Get(ctx, &GetRequest{AppName: "app", UserID: "user", SessionID: "s1"})
	require.NoError(t, err)
	require.Equal(t, 1, resp.Session.Events().Len())
	assert.Equal(t, "hello", resp.Session.Events().At(0).Text())

	val, err := resp.Session.State().Get("greeting")
	require.NoError(t, err)
	assert.Equal(t, "hi", val)
	val, err = resp.Session.State().Get("count")
	require.NoError(t, err)
	assert.Equal(t, 1, val)
	_, err = resp.Session.State().Get("partial-key")
	assert.True(t, errors.Is(err, ErrStateKeyNotExist))
}

func TestStateSetGet(t *testing.T) {
	ctx := context.Background()
	svc := InMemoryService()

	resp, err := svc.Create(ctx, &CreateRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "s1",
		State:     map[string]any{"seed": "value"},
	})
	require.NoError(t, err)

	val, err := resp.Session.State().Get("seed")
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	_, err = resp.Session.State().Get("missing")
	assert.True(t, errors.Is(err, ErrStateKeyNotExist))

	require.NoError(t, resp.Session.State().Set("k", 42))
	val, err = resp.Session.State().Get("k")
	require.NoError(t, err)
	assert.Equal(t, 42, val)

	collected := make(map[string]any)
	for k, v := range resp.Session.State().All() {
		collected[k] = v
	}
	assert.Equal(t, map[string]any{"seed": "value", "k": 42}, collected)
}
