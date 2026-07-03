package pathguard

import (
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

func TestAllowed(t *testing.T) {
	dir := resolvedTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("x"), 0o644))

	tests := []struct {
		name     string
		allowed  []string
		path     string
		expected bool
	}{
		{
			name:     "empty list denies all",
			allowed:  nil,
			path:     dir,
			expected: false,
		},
		{
			name:     "exact path match",
			allowed:  []string{dir},
			path:     dir,
			expected: true,
		},
		{
			name:     "glob star match file",
			allowed:  []string{filepath.Join(dir, "*")},
			path:     filepath.Join(dir, "file.txt"),
			expected: true,
		},
		{
			name:     "directory prefix match",
			allowed:  []string{filepath.Join(dir, "sub")},
			path:     filepath.Join(dir, "sub", "file.txt"),
			expected: true,
		},
		{
			name:     "no match",
			allowed:  []string{filepath.Join(dir, "other")},
			path:     filepath.Join(dir, "sub", "file.txt"),
			expected: false,
		},
		{
			name:     "basename match",
			allowed:  []string{"*.go"},
			path:     filepath.Join(dir, "main.go"),
			expected: true,
		},
		{
			name:     "basename no match",
			allowed:  []string{"*.go"},
			path:     filepath.Join(dir, "readme.txt"),
			expected: false,
		},
		{
			name:     "recursive star star matches nested file",
			allowed:  []string{dir + "/**"},
			path:     filepath.Join(dir, "sub", "deep", "file.txt"),
			expected: true,
		},
		{
			name:     "recursive star star matches direct child",
			allowed:  []string{dir + "/**"},
			path:     filepath.Join(dir, "file.txt"),
			expected: true,
		},
		{
			name:     "recursive star matches one level deep",
			allowed:  []string{dir + "/*"},
			path:     filepath.Join(dir, "sub", "file.txt"),
			expected: true,
		},
		{
			name:     "recursive star star no match outside dir",
			allowed:  []string{dir + "/**"},
			path:     filepath.Join(dir, "..", "other", "file.txt"),
			expected: false,
		},
		{
			name:     "recursive star star matches dir itself",
			allowed:  []string{dir + "/**"},
			path:     dir,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(tt.allowed)
			assert.Equal(t, tt.expected, g.Allowed(tt.path))
		})
	}
}

func TestCheck(t *testing.T) {
	dir := resolvedTempDir(t)

	g := New([]string{filepath.Join(dir, "*")})

	require.NoError(t, g.Check(filepath.Join(dir, "test.txt")))

	assert.Error(t, g.Check("/forbidden/path"))
}

func TestAllowedSymlink(t *testing.T) {
	dir := resolvedTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real", "file.txt"), []byte("x"), 0o644))

	linkDir := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(filepath.Join(dir, "real"), linkDir))

	g := New([]string{filepath.Join(dir, "link") + "/**"})

	assert.True(t, g.Allowed(filepath.Join(linkDir, "file.txt")))

	g2 := New([]string{filepath.Join(dir, "real") + "/**"})
	assert.True(t, g2.Allowed(filepath.Join(linkDir, "file.txt")))
}
