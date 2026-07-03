package listdir

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return resolved
}

func setupDir(t *testing.T) string {
	t.Helper()
	dir := resolvedTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hi"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.go"), []byte("pkg"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "nested.txt"), []byte("nested"), 0o644))
	return dir
}

func TestListDir_NonRecursive(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path": dir,
	})
	require.NoError(t, err)

	entries := result["entries"].([]dirEntry)
	assert.Equal(t, 3, len(entries))
	names := map[string]string{}
	for _, e := range entries {
		names[e.Name] = e.Type
	}
	assert.Equal(t, "file", names["file1.txt"])
	assert.Equal(t, "file", names["file2.go"])
	assert.Equal(t, "dir", names["subdir"])
}

func TestListDir_Recursive(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":      dir,
		"recursive": true,
	})
	require.NoError(t, err)

	entries := result["entries"].([]dirEntry)
	assert.GreaterOrEqual(t, len(entries), 4)
}

func TestListDir_NonexistentDirectory(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/**"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path": "/nonexistent_dir_12345",
	})
	assert.Error(t, err)
}

func TestListDir_PathGuard(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/noaccess/*"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path": "/forbidden",
	})
	assert.Error(t, err)
}

func TestListDir_EmptyDirectory(t *testing.T) {
	dir := resolvedTempDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path": dir,
	})
	require.NoError(t, err)

	entries := result["entries"].([]dirEntry)
	assert.Empty(t, entries)
}
