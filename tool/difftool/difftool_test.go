package difftool

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

func TestUnifiedDiff(t *testing.T) {
	tests := []struct {
		name     string
		old      string
		new      string
		empty    bool
		contains string
	}{
		{
			name:  "identical content",
			old:   "hello\nworld\n",
			new:   "hello\nworld\n",
			empty: true,
		},
		{
			name:     "single insertion",
			old:       "a\nb\n",
			new:       "a\nx\nb\n",
			contains: "+x",
		},
		{
			name:     "single deletion",
			old:       "a\nb\nc\n",
			new:       "a\nc\n",
			contains: "-b",
		},
		{
			name:     "mixed changes",
			old:       "a\nb\nc\n",
			new:       "a\nx\nc\n",
			contains:  "-b",
		},
		{
			name:     "empty to content",
			old:       "",
			new:       "hello\n",
			contains: "+hello",
		},
		{
			name:     "content to empty",
			old:       "hello\n",
			new:       "",
			contains: "-hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unifiedDiff(tt.old, tt.new, 3)
			if tt.empty {
				assert.Empty(t, result)
			} else {
				assert.Contains(t, result, tt.contains)
			}
		})
	}
}

func TestProcess(t *testing.T) {
	dir := resolvedTempDir(t)
	filePath := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	tool, err := New(Config{AllowedPaths: []string{filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":        filePath,
		"new_content": "line1\nmodified\nline3\n",
	})
	require.NoError(t, err)
	assert.True(t, result["has_changes"].(bool))
	assert.Contains(t, result["diff"].(string), "-line2")
	assert.Contains(t, result["diff"].(string), "+modified")
}

func TestProcess_IdenticalContent(t *testing.T) {
	dir := resolvedTempDir(t)
	filePath := filepath.Join(dir, "same.txt")
	content := "same content\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	tool, err := New(Config{AllowedPaths: []string{filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"path":        filePath,
		"new_content": content,
	})
	require.NoError(t, err)
	assert.False(t, result["has_changes"].(bool))
}

func TestProcess_FileNotFound(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/**"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path":        "/nonexistent/file.txt",
		"new_content": "x",
	})
	assert.Error(t, err)
}

func TestProcess_PathGuard(t *testing.T) {
	dir := resolvedTempDir(t)
	filePath := filepath.Join(dir, "blocked.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("content\n"), 0o644))

	tool, err := New(Config{AllowedPaths: []string{"/noaccess/*"}})
	require.NoError(t, err)

	_, err = tool.Process(context.Background(), map[string]any{
		"path":        filePath,
		"new_content": "x",
	})
	assert.Error(t, err)
}
