package memory

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/session"
)

func seedSession(t *testing.T, svc session.Service, appName, userID, sessionID string, texts ...string) session.Session {
	t.Helper()
	ctx := context.Background()
	resp, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	require.NoError(t, err)
	for _, text := range texts {
		event := session.NewEvent("inv")
		event.Author = "model"
		event.ModelResponse = content.NewTextContent(text, content.RoleModel)
		require.NoError(t, svc.AppendEvent(ctx, resp.Session, event))
	}
	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	require.NoError(t, err)
	return getResp.Session
}

func search(t *testing.T, svc Service, req *SearchRequest) []Entry {
	t.Helper()
	resp, err := svc.SearchMemory(context.Background(), req)
	require.NoError(t, err)
	return resp.Memories
}

func TestSearchMemoryCrossUserIsolation(t *testing.T) {
	ctx := context.Background()
	sessSvc := session.InMemoryService()
	svc := InMemoryService()

	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app", "alice", "s1", "user alice likes cats")))
	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app", "alice", "s2", "user alice likes dogs")))
	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app", "bob", "s3", "user alice likes birds")))

	results := search(t, svc, &SearchRequest{Query: "alice", UserID: "alice", AppName: "app"})
	assert.Len(t, results, 2)
	for _, e := range results {
		assert.Equal(t, "alice", e.UserID)
		assert.Equal(t, "app", e.AppName)
	}

	results = search(t, svc, &SearchRequest{Query: "alice", UserID: "bob", AppName: "app"})
	require.Len(t, results, 1)
	assert.Equal(t, "user alice likes birds", results[0].Content)
	assert.Equal(t, "bob", results[0].UserID)
}

func TestSearchMemoryCrossAppIsolation(t *testing.T) {
	ctx := context.Background()
	sessSvc := session.InMemoryService()
	svc := InMemoryService()

	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app1", "alice", "s1", "shared secret from app1")))
	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app2", "alice", "s2", "shared secret from app2")))

	results := search(t, svc, &SearchRequest{Query: "shared secret", AppName: "app1"})
	require.Len(t, results, 1)
	assert.Equal(t, "app1", results[0].AppName)
	assert.Equal(t, "shared secret from app1", results[0].Content)
}

func TestSearchMemoryEmptyScopeMatchesAll(t *testing.T) {
	ctx := context.Background()
	sessSvc := session.InMemoryService()
	svc := InMemoryService()

	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app1", "alice", "s1", "hello world")))
	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app2", "bob", "s2", "hello there")))

	results := search(t, svc, &SearchRequest{Query: "hello"})
	assert.Len(t, results, 2)
}

func TestSearchMemorySubstringMatching(t *testing.T) {
	ctx := context.Background()
	sessSvc := session.InMemoryService()
	svc := InMemoryService()

	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app", "alice", "s1", "The Quick Brown Fox jumps")))
	require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app", "alice", "s2", "lazy dog sleeps")))

	results := search(t, svc, &SearchRequest{Query: "quick brown", AppName: "app", UserID: "alice"})
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "Quick Brown Fox")

	assert.Empty(t, search(t, svc, &SearchRequest{Query: "zebra"}))
}

func TestInMemoryServiceMaxEntriesEvictsOldest(t *testing.T) {
	ctx := context.Background()
	sessSvc := session.InMemoryService()
	svc := InMemoryService(WithMaxEntries(3))

	for i := 1; i <= 5; i++ {
		text := fmt.Sprintf("memory entry %d", i)
		require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app", "alice", fmt.Sprintf("s%d", i), text)))
	}

	results := search(t, svc, &SearchRequest{})
	require.Len(t, results, 3)
	want := []string{"memory entry 3", "memory entry 4", "memory entry 5"}
	for i, e := range results {
		assert.Equal(t, want[i], e.Content)
	}
}

func TestInMemoryServiceZeroArgIsUnbounded(t *testing.T) {
	ctx := context.Background()
	sessSvc := session.InMemoryService()
	svc := InMemoryService()

	const total = 100
	for i := 0; i < total; i++ {
		require.NoError(t, svc.AddSessionToMemory(ctx, seedSession(t, sessSvc, "app", "alice", fmt.Sprintf("s%d", i), "entry")))
	}

	results := search(t, svc, &SearchRequest{})
	assert.Len(t, results, total)
}
