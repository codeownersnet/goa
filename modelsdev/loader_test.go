package modelsdev_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/modelsdev"
)

const syntheticCatalog = `{
	"$schema": "https://models.dev/schema.json",
	"testprovider": {
		"id": "testprovider",
		"name": "Test Provider",
		"api": "https://api.testprovider.test/v1",
		"env": ["TESTPROVIDER_API_KEY"],
		"models": [
			{
				"id": "test-model",
				"name": "Test Model",
				"reasoning": false,
				"tool_call": true,
				"structured_output": true,
				"attachment": false,
				"temperature": true,
				"modalities": {"input": ["text"], "output": ["text"]},
				"cost": {"input": 1.0, "output": 2.0},
				"limit": {"context": 128000, "output": 4096}
			}
		]
	}
}`

func TestParseCatalog(t *testing.T) {
	catalog, err := modelsdev.ParseCatalog([]byte(syntheticCatalog))
	require.NoError(t, err)
	require.Len(t, catalog.Providers, 1)

	p := catalog.Providers[0]
	assert.Equal(t, "testprovider", p.ID)
	assert.Equal(t, "Test Provider", p.Name)
	assert.Equal(t, "https://api.testprovider.test/v1", p.API)
	assert.Equal(t, []string{"TESTPROVIDER_API_KEY"}, p.Env)
	require.Len(t, p.Models, 1)

	m := p.Models[0]
	assert.Equal(t, "test-model", m.ID)
	assert.True(t, m.ToolCall)
	assert.True(t, m.StructuredOutput)
	assert.False(t, m.Reasoning)
	assert.Equal(t, 1.0, m.Cost.Input)
	assert.Equal(t, 128000, m.Limit.Context)
	assert.Equal(t, []string{"text"}, m.Modalities.Input)

	invalid, err := modelsdev.ParseCatalog([]byte("{not json"))
	assert.Error(t, err)
	assert.Nil(t, invalid)
}

func TestLoadCatalogCustomURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(syntheticCatalog))
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())

	catalog, err := modelsdev.LoadCatalog(context.Background(), modelsdev.WithCustomURL(server.URL))
	require.NoError(t, err)
	require.Len(t, catalog.Providers, 1)
	assert.Equal(t, "testprovider", catalog.Providers[0].ID)
}

func TestLoadCatalogOfflineEmbedded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	catalog, err := modelsdev.LoadCatalog(context.Background(),
		modelsdev.WithOffline(),
	)
	require.NoError(t, err)
	assert.NotEmpty(t, catalog.Providers)
}

func TestLoadCatalogWritesHomeCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(syntheticCatalog))
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := modelsdev.LoadCatalog(context.Background(), modelsdev.WithCustomURL(server.URL))
	require.NoError(t, err)

	cachePath := filepath.Join(home, ".goa", "catalog.json")
	info, err := os.Stat(cachePath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Greater(t, info.Size(), int64(0))
}
