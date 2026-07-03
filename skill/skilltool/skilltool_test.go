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

func createSkillWithResource(t *testing.T) (*skill.Registry, string) {
	t.Helper()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "res-skill")
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "references"), 0o755))

	skillMd := `---
name: res-skill
description: Resource skill.
---
# Instructions
Do things.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "references", "data.txt"), []byte("resource content"), 0o644))

	reg := skill.NewRegistry(skill.WithSkillDirs(dir))
	require.NoError(t, reg.Discover())

	_, err := reg.Activate("res-skill")
	require.NoError(t, err)
	return reg, dir
}

func TestResourceTool_ReadResource(t *testing.T) {
	reg, _ := createSkillWithResource(t)
	tool := NewResourceTool(reg)

	result, err := tool.Process(context.Background(), map[string]any{
		"name":           "res-skill",
		"relative_path": "data.txt",
	})
	require.NoError(t, err)
	assert.Equal(t, "resource content", result["content"])
}

func TestResourceTool_NonStringName(t *testing.T) {
	reg, _ := createSkillWithResource(t)
	tool := NewResourceTool(reg)

	_, err := tool.Process(context.Background(), map[string]any{
		"name":           123,
		"relative_path": "data.txt",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResourceTool_SkillNotFound(t *testing.T) {
	reg, _ := createSkillWithResource(t)
	tool := NewResourceTool(reg)

	_, err := tool.Process(context.Background(), map[string]any{
		"name":           "nonexistent",
		"relative_path": "x.txt",
	})
	assert.Error(t, err)
}

func TestResourceTool_PathTraversal(t *testing.T) {
	reg, _ := createSkillWithResource(t)
	tool := NewResourceTool(reg)

	_, err := tool.Process(context.Background(), map[string]any{
		"name":           "res-skill",
		"relative_path": "../etc/passwd",
	})
	assert.Error(t, err)
}

func TestResourceTool_Name(t *testing.T) {
	tool := NewResourceTool(nil)
	assert.Equal(t, "read_skill_resource", tool.Name())
}

func TestResourceTool_Declaration(t *testing.T) {
	tool := NewResourceTool(nil)
	decl := tool.Declaration()
	assert.Equal(t, "read_skill_resource", decl.Name)
	assert.NotNil(t, decl.Parameters)
}

func TestScriptTool_NonStringName(t *testing.T) {
	tool := NewScriptTool(nil)

	_, err := tool.Process(context.Background(), map[string]any{
		"name":        42,
		"script_path": "run.sh",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestScriptTool_NonStringScriptPath(t *testing.T) {
	tool := NewScriptTool(nil)

	_, err := tool.Process(context.Background(), map[string]any{
		"name":        "skill",
		"script_path": 42,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestScriptTool_Name(t *testing.T) {
	tool := NewScriptTool(nil)
	assert.Equal(t, "run_skill_script", tool.Name())
}

func TestScriptTool_Declaration(t *testing.T) {
	tool := NewScriptTool(nil)
	decl := tool.Declaration()
	assert.Equal(t, "run_skill_script", decl.Name)
	assert.NotNil(t, decl.Parameters)
}

func TestScriptTool_ScriptsDisabled(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "script-skill")
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755))

	skillMd := `---
name: script-skill
description: Script skill.
---
# Instructions
Do things.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))

	reg := skill.NewRegistry(skill.WithSkillDirs(dir), skill.WithRunScripts(false))
	require.NoError(t, reg.Discover())
	_, err := reg.Activate("script-skill")
	require.NoError(t, err)

	tool := NewScriptTool(reg)
	_, err = tool.Process(context.Background(), map[string]any{
		"name":        "script-skill",
		"script_path": "run.sh",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}
