package writefile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFileEnsuresTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	tool, err := New(Config{AllowedPaths: []string{dir + "/**"}})
	require.NoError(t, err)

	_, err = tool.Process(nil, map[string]any{
		"path":    path,
		"content": "hello world",
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", string(data))
}

func TestWriteFilePreservesExistingTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	tool, err := New(Config{AllowedPaths: []string{dir + "/**"}})
	require.NoError(t, err)

	_, err = tool.Process(nil, map[string]any{
		"path":    path,
		"content": "hello world\n",
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", string(data))
}

func TestWriteFileEmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	tool, err := New(Config{AllowedPaths: []string{dir + "/**"}})
	require.NoError(t, err)

	_, err = tool.Process(nil, map[string]any{
		"path":    path,
		"content": "",
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "\n", string(data))
}
