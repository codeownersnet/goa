package registry

import (
	"context"
	"testing"

	"github.com/codeownersnet/goa/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	r := New()
	assert.NotNil(t, r)
	assert.Empty(t, r.Names())
}

func TestNewWithOptions(t *testing.T) {
	r := New(
		WithFactory("tool1", func() (tool.Tool, error) { return &mockTool{name: "tool1"}, nil }),
	)
	assert.True(t, r.Has("tool1"))
	assert.False(t, r.Has("tool2"))
}

func TestRegister(t *testing.T) {
	r := New()
	r.Register("my_tool", func() (tool.Tool, error) { return &mockTool{name: "my_tool"}, nil })
	assert.True(t, r.Has("my_tool"))
}

func TestLookup(t *testing.T) {
	r := New()
	r.Register("my_tool", func() (tool.Tool, error) { return &mockTool{name: "my_tool"}, nil })

	got, err := r.Lookup("my_tool")
	require.NoError(t, err)
	assert.Equal(t, "my_tool", got.Name())
}

func TestLookupNotFound(t *testing.T) {
	r := New()
	_, err := r.Lookup("nonexistent")
	assert.ErrorIs(t, err, ErrToolNotFound)
}

func TestLookupFactoryError(t *testing.T) {
	r := New()
	r.Register("broken", func() (tool.Tool, error) {
		return nil, assert.AnError
	})
	_, err := r.Lookup("broken")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
}

func TestNames(t *testing.T) {
	r := New()
	r.Register("a", func() (tool.Tool, error) { return &mockTool{name: "a"}, nil })
	r.Register("b", func() (tool.Tool, error) { return &mockTool{name: "b"}, nil })
	names := r.Names()
	assert.Len(t, names, 2)
}

func TestDefaultBuiltinRegistry(t *testing.T) {
	r := DefaultBuiltinRegistry()
	expected := []string{"bash", "read_file", "write_file", "edit_file", "glob", "grep", "list_dir", "exit_loop", "diff"}
	for _, name := range expected {
		assert.True(t, r.Has(name), "expected %q in default registry", name)
		tl, err := r.Lookup(name)
		require.NoError(t, err, "lookup %q should succeed", name)
		assert.Equal(t, name, tl.Name(), "tool name should match %q", name)
	}
}

func TestDefaultBuiltinRegistryWithAllowedPaths(t *testing.T) {
	paths := []string{"/tmp/test/**"}
	r := DefaultBuiltinRegistry(WithBuiltinAllowedPaths(paths))
	assert.NotNil(t, r)
	assert.True(t, r.Has("glob"))
}

type mockTool struct {
	name string
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string  { return "mock" }
func (m *mockTool) Process(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, nil
}
