package bash

import (
	"context"
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

func TestExtractCommands(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected []string
	}{
		{
			name:     "simple command",
			cmd:      "git status",
			expected: []string{"git"},
		},
		{
			name:     "compound with &&",
			cmd:      "git log && git diff",
			expected: []string{"git"},
		},
		{
			name:     "piped commands",
			cmd:      "echo hello | grep h",
			expected: []string{"echo", "grep"},
		},
		{
			name:     "compound with ;",
			cmd:      "cd /tmp ; ls -la",
			expected: []string{"cd", "ls"},
		},
		{
			name:     "empty string",
			cmd:      "",
			expected: nil,
		},
		{
			name:     "complex chain",
			cmd:      "cat file | grep foo | wc -l",
			expected: []string{"cat", "grep", "wc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCommands(tt.cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNew_DeniedCommand(t *testing.T) {
	tool, err := New(Config{DeniedCommands: []string{"rm"}})
	require.NoError(t, err)

	_, err = tool.Tool.Process(context.Background(), map[string]any{
		"command": "rm -rf /",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

func TestNew_AllowedCommand(t *testing.T) {
	tool, err := New(Config{AllowedCommands: []string{"echo"}})
	require.NoError(t, err)

	_, err = tool.Tool.Process(context.Background(), map[string]any{
		"command": "git status",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in the allowed list")
}

func TestProcess_Echo(t *testing.T) {
	tool, err := New(Config{})
	require.NoError(t, err)

	result, err := tool.Tool.Process(context.Background(), map[string]any{
		"command": "echo hello",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"].(string), "hello")
}

func TestProcess_NonZeroExitCode(t *testing.T) {
	tool, err := New(Config{})
	require.NoError(t, err)

	_, err = tool.Tool.Process(context.Background(), map[string]any{
		"command": "sh -c 'exit 1'",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bash:")
}

func TestProcess_WorkDir(t *testing.T) {
	dir := resolvedTempDir(t)
	tool, err := New(Config{AllowedPaths: []string{dir, filepath.Join(dir, "*")}})
	require.NoError(t, err)

	result, err := tool.Tool.Process(context.Background(), map[string]any{
		"command": "pwd",
		"workdir": dir,
	})
	require.NoError(t, err)
	assert.Contains(t, result["stdout"].(string), dir)
}

func TestProcess_PathGuard(t *testing.T) {
	tool, err := New(Config{AllowedPaths: []string{"/noaccess/*"}})
	require.NoError(t, err)

	_, err = tool.Tool.Process(context.Background(), map[string]any{
		"command": "pwd",
		"workdir": "/forbidden",
	})
	assert.Error(t, err)
}
