package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/tool"
)

type resultPolicyContext struct {
	tool.Context
	sessionID    string
	invocationID string
}

func (c resultPolicyContext) SessionID() string    { return c.sessionID }
func (c resultPolicyContext) InvocationID() string { return c.invocationID }

func TestTruncateResultPolicyLeavesSmallResultsUntouched(t *testing.T) {
	policy := NewTruncateResultPolicy(TruncatePolicyConfig{MaxBytes: 1024})
	result := map[string]any{"ok": true, "value": "small"}

	processed, err := policy.Process(resultPolicyContext{}, result)
	require.NoError(t, err)
	assert.Equal(t, result, processed)
}

func TestTruncateResultPolicyBoundsNestedResults(t *testing.T) {
	policy := NewTruncateResultPolicy(TruncatePolicyConfig{MaxBytes: 512})
	result := map[string]any{
		"nested": map[string]any{"value": strings.Repeat("x", 4000)},
		"list":   []any{strings.Repeat("y", 4000)},
	}

	processed, err := policy.Process(resultPolicyContext{}, result)
	require.NoError(t, err)
	encoded, err := json.Marshal(processed)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(encoded), 512)
	assert.Equal(t, true, processed["_output_truncated"])
	assert.NotContains(t, processed, "_output_path")
}

func TestTruncateResultPolicySpillsPerRun(t *testing.T) {
	root := t.TempDir()
	policy := NewTruncateResultPolicy(TruncatePolicyConfig{MaxBytes: 256, SpillDir: root})
	big := strings.Repeat("z", 4000)
	result := map[string]any{"value": big}

	first, err := policy.Process(resultPolicyContext{sessionID: "session-1", invocationID: "run-1"}, result)
	require.NoError(t, err)
	firstPath, ok := first["_output_path"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(firstPath, root))
	data, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), big)

	second, err := policy.Process(resultPolicyContext{sessionID: "session-1", invocationID: "run-2"}, result)
	require.NoError(t, err)
	secondPath, ok := second["_output_path"].(string)
	require.True(t, ok)
	assert.NotEqual(t, filepath.Dir(firstPath), filepath.Dir(secondPath))
}

func TestTruncateResultPolicyCleansOldRunDirs(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "0123456789abcdef")
	require.NoError(t, os.MkdirAll(oldDir, 0o700))
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

	policy := NewTruncateResultPolicy(TruncatePolicyConfig{MaxBytes: 128, SpillDir: root})
	_, err := policy.Process(resultPolicyContext{sessionID: "s", invocationID: "i"}, map[string]any{"value": strings.Repeat("x", 1000)})
	require.NoError(t, err)

	_, err = os.Stat(oldDir)
	assert.True(t, os.IsNotExist(err))
}
