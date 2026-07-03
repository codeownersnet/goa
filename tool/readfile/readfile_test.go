package readfile

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

func writeFile(t *testing.T, content string) (dir, path string) {
	t.Helper()
	dir = resolvedTempDir(t)
	path = filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return
}

func TestReadFile_FullFile(t *testing.T) {
	dir, path := writeFile(t, "line1\nline2\nline3\n")
	tool, err := New(Config{AllowedPaths: []string{filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path": path,
	})
	require.NoError(t, err)

	content := result["content"].(string)
	assert.Contains(t, content, "0: line1")
	assert.Contains(t, content, "1: line2")
	assert.Contains(t, content, "2: line3")
}

func TestReadFile_WithOffset(t *testing.T) {
	dir, path := writeFile(t, "line1\nline2\nline3\nline4\nline5\n")
	tool, err := New(Config{AllowedPaths: []string{filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":   path,
		"offset": float64(3),
	})
	require.NoError(t, err)

	content := result["content"].(string)
	assert.Contains(t, content, "3: line3")
	assert.Contains(t, content, "4: line4")
	assert.NotContains(t, content, "line1")
	assert.NotContains(t, content, "line2")
}

func TestReadFile_WithLimit(t *testing.T) {
	dir, path := writeFile(t, "line1\nline2\nline3\nline4\nline5\n")
	tool, err := New(Config{AllowedPaths: []string{filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":  path,
		"limit": float64(2),
	})
	require.NoError(t, err)

	content := result["content"].(string)
	assert.Contains(t, content, "0: line1")
	assert.Contains(t, content, "1: line2")
	assert.NotContains(t, content, "line3")
}

func TestReadFile_WithOffsetAndLimit(t *testing.T) {
	dir, path := writeFile(t, "line1\nline2\nline3\nline4\nline5\n")
	tool, err := New(Config{AllowedPaths: []string{filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":   path,
		"offset": float64(2),
		"limit":  float64(2),
	})
	require.NoError(t, err)

	content := result["content"].(string)
	assert.Contains(t, content, "2: line2")
	assert.Contains(t, content, "3: line3")
	assert.NotContains(t, content, "line1")
	assert.NotContains(t, content, "line4")
}

func TestReadFile_MaxBytes(t *testing.T) {
	dir, path := writeFile(t, "x\n")
	tool, err := New(Config{AllowedPaths: []string{filepath.Join(dir, "*")}, MaxBytes: 1})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path": path,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds max")
}

func TestReadFile_FileNotFound(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/**"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path": "/nonexistent_file.txt",
	})
	assert.Error(t, err)
}

func TestReadFile_PathGuard(t *testing.T) {
	_, path := writeFile(t, "content\n")
	tool, err := New(Config{AllowedPaths: []string{"/noaccess/*"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path": path,
	})
	assert.Error(t, err)
}
