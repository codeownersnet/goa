package skilltool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codeownersnet/goa/skill"
)

func TestActivateToolReturnsInstructions(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	skillMd := `---
name: my-skill
description: Test skill.
---
# Instructions
Do the thing.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))

	reg := skill.NewRegistry(skill.WithSkillDirs(dir))
	require.NoError(t, reg.Discover())

	tool := NewActivateTool(reg)
	result, err := tool.Process(context.Background(), map[string]any{"name": "my-skill"})
	require.NoError(t, err)

	assert.Equal(t, "my-skill", result["name"])
	assert.Equal(t, "Test skill.", result["description"])
	assert.Contains(t, result["instructions"], "# Instructions")
}
