package glob

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
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("go"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("txt"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "c.go"), []byte("go"), 0o644))
	return dir
}

func TestGlob(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "*.go",
	})
	require.NoError(t, err)

	files := result["files"].([]string)
	assert.Equal(t, 1, len(files))
	assert.Contains(t, files[0], "a.go")
}

func TestGlob_NoMatches(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "*.rs",
	})
	require.NoError(t, err)

	files := result["files"].([]string)
	assert.Empty(t, files)
}

func TestGlob_MaxResults(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}, MaxResults: 1})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "*",
	})
	require.NoError(t, err)

	files := result["files"].([]string)
	assert.Len(t, files, 1)
}

func TestGlob_PathGuard(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/noaccess/*"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path":    "/forbidden",
		"pattern": "*",
	})
	assert.Error(t, err)
}
