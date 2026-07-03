package grep

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc hello() {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello world\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "c.go"), []byte("package sub\nfunc bar() {}\n"), 0o644))
	return dir
}

func TestGrep_BasicSearch(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "hello",
	})
	require.NoError(t, err)

	count := result["count"].(int)
	assert.GreaterOrEqual(t, count, 2)
	assert.Contains(t, result["output"].(string), "Found")
}

func TestGrep_NoMatches(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "zzz_nonexistent",
	})
	require.NoError(t, err)

	count := result["count"].(int)
	assert.Equal(t, 0, count)
	assert.Contains(t, result["output"].(string), "Found 0 matches")
}

func TestGrep_IncludeFilter(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "hello",
		"include": "*.go",
	})
	require.NoError(t, err)

	count := result["count"].(int)
	assert.Equal(t, 1, count)
}

func TestGrep_MaxResults(t *testing.T) {
	dir := setupDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}, MaxResults: 1})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "func|hello",
	})
	require.NoError(t, err)

	count := result["count"].(int)
	assert.Equal(t, 1, count)
	output := result["output"].(string)
	assert.Contains(t, output, "showing first")
}

func TestGrep_InvalidRegex(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/**"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path":    "/tmp",
		"pattern": "[invalid",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex")
}

func TestGrep_PathGuard(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/noaccess/*"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path":    "/forbidden",
		"pattern": "test",
	})
	assert.Error(t, err)
}
