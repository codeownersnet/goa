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
		{
			name:     "or chain",
			cmd:      "make build || make clean",
			expected: []string{"make"},
		},
		{
			name:     "newline split",
			cmd:      "cd /tmp\nls -la\r\npwd",
			expected: []string{"cd", "ls", "pwd"},
		},
		{
			name:     "dollar substitution extracted",
			cmd:      "echo $(rm x)",
			expected: []string{"rm", "echo"},
		},
		{
			name:     "backtick substitution extracted",
			cmd:      "echo `rm x`",
			expected: []string{"rm", "echo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractCommands(tt.cmd)
			require.NoError(t, err)
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

func TestProcess_ConfigWorkDirWithoutAllowedPaths(t *testing.T) {
	dir := resolvedTempDir(t)
	tool, err := New(Config{WorkDir: dir})
	require.NoError(t, err)

	result, err := tool.Tool.Process(context.Background(), map[string]any{
		"command": "pwd",
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

func TestNew_AllowlistRejectsShellInvoker(t *testing.T) {
	tool, err := New(Config{AllowedCommands: []string{"go"}})
	require.NoError(t, err)

	_, err = tool.Tool.Process(context.Background(), map[string]any{
		"command": `sh -c "go version"`,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in the allowed list")
}

func TestNew_AllowlistRejectsCommandSubstitution(t *testing.T) {
	for _, cmd := range []string{"echo $(rm x)", "echo `rm x`"} {
		tool, err := New(Config{AllowedCommands: []string{"echo"}})
		require.NoError(t, err)

		_, err = tool.Tool.Process(context.Background(), map[string]any{
			"command": cmd,
		})
		assert.Error(t, err, "command %q should be rejected", cmd)
		assert.Contains(t, err.Error(), "not in the allowed list")
	}
}

func TestNew_DenyListRejectsCommandSubstitution(t *testing.T) {
	for _, cmd := range []string{"echo $(rm x)", "echo `rm x`"} {
		tool, err := New(Config{DeniedCommands: []string{"rm"}})
		require.NoError(t, err)

		_, err = tool.Tool.Process(context.Background(), map[string]any{
			"command": cmd,
		})
		assert.Error(t, err, "command %q should be rejected", cmd)
		assert.Contains(t, err.Error(), "denied")
	}
}

func TestNew_OrChainRequiresBothAllowed(t *testing.T) {
	tool, err := New(Config{AllowedCommands: []string{"true", "echo"}})
	require.NoError(t, err)

	result, err := tool.Tool.Process(context.Background(), map[string]any{
		"command": "true || echo fallback",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	blocked, err := New(Config{AllowedCommands: []string{"true"}})
	require.NoError(t, err)

	_, err = blocked.Tool.Process(context.Background(), map[string]any{
		"command": "true || echo fallback",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in the allowed list")
}

func TestProcess_ConfigWorkDirGuarded(t *testing.T) {
	dir := resolvedTempDir(t)
	tool, err := New(Config{
		AllowedPaths: []string{dir + "/**"},
		WorkDir:      "/forbidden-workdir",
	})
	require.NoError(t, err)

	_, err = tool.Tool.Process(context.Background(), map[string]any{
		"command": "pwd",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in the allowed list")
}

func TestNew_AllowedHappyPath(t *testing.T) {
	tool, err := New(Config{AllowedCommands: []string{"go"}})
	require.NoError(t, err)

	result, err := tool.Tool.Process(context.Background(), map[string]any{
		"command": "go version",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"].(string), "go version")
}

func TestNew_UnbalancedQuoteRejectedWithAllowlist(t *testing.T) {
	tool, err := New(Config{AllowedCommands: []string{"echo"}})
	require.NoError(t, err)

	_, err = tool.Tool.Process(context.Background(), map[string]any{
		"command": `echo "unbalanced`,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse command safely")
}
