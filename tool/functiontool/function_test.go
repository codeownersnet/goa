package functiontool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/tool"
)

func TestNew_InvalidTypes(t *testing.T) {
	_, err := New[int, int](Config{Name: "bad"}, nil)
	assert.Error(t, err)

	_, err = New[string, string](Config{Name: "bad"}, nil)
	assert.Error(t, err)

	_, err = New[[]int, string](Config{Name: "bad"}, nil)
	assert.Error(t, err)
}

func TestNew_ValidTypes(t *testing.T) {
	type args struct{ Name string }
	type result struct{ Output string }

	_, err := New[args, result](Config{Name: "valid"}, nil)
	assert.NoError(t, err)

	_, err = New[*args, result](Config{Name: "valid_ptr"}, nil)
	assert.NoError(t, err)

	_, err = New[map[string]any, result](Config{Name: "valid_map"}, nil)
	assert.NoError(t, err)
}

func TestName(t *testing.T) {
	type args struct{ X int }
	type result struct{}

	tool, err := New[args, result](Config{Name: "my_tool", Description: "does things"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "my_tool", tool.Name())
	assert.Equal(t, "does things", tool.Description())
}

func TestProcess_RoundTrip(t *testing.T) {
	type args struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	type result struct {
		Greeting string `json:"greeting"`
	}

	tool, err := New[args, result](Config{Name: "greet"}, func(_ context.Context, a args) (result, error) {
		return result{Greeting: "hello " + a.Name}, nil
	})
	require.NoError(t, err)

	out, err := tool.Process(context.Background(), map[string]any{
		"name": "world",
		"age":  float64(42),
	})
	require.NoError(t, err)
	assert.Equal(t, "hello world", out["greeting"])
}

func TestProcess_HandlerError(t *testing.T) {
	type args struct{}
	type result struct{}

	tool, err := New[args, result](Config{Name: "fail"}, func(_ context.Context, _ args) (result, error) {
		return result{}, assert.AnError
	})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{})
	assert.ErrorIs(t, err, assert.AnError)
}

func TestProcess_MapResult(t *testing.T) {
	type args struct{}
	tool, err := New[args, map[string]any](Config{Name: "map_result"}, func(_ context.Context, _ args) (map[string]any, error) {
		return map[string]any{"key": "value"}, nil
	})
	require.NoError(t, err)

	out, err := tool.Process(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "value", out["key"])
}

func TestDeclaration(t *testing.T) {
	type args struct {
		Name string `json:"name"`
	}
	tl, err := New[args, any](Config{Name: "decl_tool", Description: "desc"}, nil)
	require.NoError(t, err)

	decl := tl.(tool.Declarer).Declaration()
	assert.Equal(t, "decl_tool", decl.Name)
	assert.Equal(t, "desc", decl.Description)
	assert.NotNil(t, decl.Parameters)
}

func TestSchemaInference(t *testing.T) {
	type args struct {
		Name    string  `json:"name"`
		Age     int     `json:"age"`
		Score   float64 `json:"score"`
		Active  bool    `json:"active"`
		Optional string `json:"opt,omitempty"`
	}
	tl, err := New[args, any](Config{Name: "schema_test"}, nil)
	require.NoError(t, err)

	decl := tl.(tool.Declarer).Declaration()
	s := decl.Parameters
	assert.Equal(t, "object", s.Type)
	assert.NotNil(t, s.Properties["name"])
	assert.NotNil(t, s.Properties["age"])
	assert.NotNil(t, s.Properties["score"])
	assert.NotNil(t, s.Properties["active"])
	assert.Contains(t, s.Required, "name")
	assert.NotContains(t, s.Required, "opt")
}
