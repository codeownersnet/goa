package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "my-skill", false},
		{"valid simple", "pdf-processing", false},
		{"valid with numbers", "golang-123", false},
		{"uppercase", "My-Skill", true},
		{"starts with hyphen", "-skill", true},
		{"ends with hyphen", "skill-", true},
		{"consecutive hyphens", "my--skill", true},
		{"empty", "", true},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"spaces", "my skill", true},
		{"special chars", "my_skill", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	name := "test-skill"
	skillDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	skillMd := `---
name: test-skill
description: A test skill for unit testing.
license: MIT
metadata:
  author: test
  version: "1.0"
allowed-tools: Bash(git:*) Read
---
# Test Skill

This is a test skill body.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))

	skill, err := Load(skillDir)
	require.NoError(t, err)
	assert.Equal(t, "test-skill", skill.Name)
	assert.Equal(t, "A test skill for unit testing.", skill.Description)
	assert.Equal(t, "MIT", skill.License)
	assert.Equal(t, map[string]string{"author": "test", "version": "1.0"}, skill.Metadata)
	assert.Equal(t, []string{"Bash(git:*)", "Read"}, skill.AllowedTools)
	assert.Equal(t, skillDir, skill.Location)
	assert.Contains(t, skill.Body, "# Test Skill")
	assert.Contains(t, skill.Body, "This is a test skill body.")
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	assert.Error(t, err)
}

func TestLoadMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "bad-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	skillMd := `---
name: bad-skill
---
No description.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))

	_, err := Load(skillDir)
	assert.Error(t, err)
}

func TestRegistryDiscover(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	skillMd := `---
name: my-skill
description: Test skill for discovery.
---
Body content.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))

	reg := NewRegistry(WithSkillDirs(dir))
	require.NoError(t, reg.Discover())

	skills := reg.List()
	assert.Len(t, skills, 1)
	assert.Equal(t, "my-skill", skills[0].Name)
	assert.Equal(t, "Test skill for discovery.", skills[0].Description)
}

func TestRegistryActivate(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	skillMd := `---
name: my-skill
description: Test skill for activation.
---
# Instructions
Do the thing.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))

	reg := NewRegistry(WithSkillDirs(dir))
	require.NoError(t, reg.Discover())

	skill, err := reg.Activate("my-skill")
	require.NoError(t, err)
	assert.Equal(t, "my-skill", skill.Name)
	assert.Contains(t, skill.Body, "# Instructions")
}

func TestRegistryReadResourceAfterActivate(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "references"), 0o755))

	skillMd := `---
name: my-skill
description: Test skill for resources.
---
# Instructions
Do the thing.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "references", "guide.md"), []byte("resource content"), 0o644))

	reg := NewRegistry(WithSkillDirs(dir))
	require.NoError(t, reg.Discover())
	_, err := reg.Activate("my-skill")
	require.NoError(t, err)

	data, err := reg.ReadResource("my-skill", "guide.md")
	require.NoError(t, err)
	assert.Equal(t, "resource content", string(data))
}

func TestDefaultSkillDirsIncludesHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dirs := defaultSkillDirs()
	assert.Contains(t, dirs, ".agents/skills")
	assert.Contains(t, dirs, ".goa/skills")
	assert.Contains(t, dirs, filepath.Join(home, ".agents/skills"))
	assert.Contains(t, dirs, filepath.Join(home, ".goa/skills"))
}

func TestNewRegistryUsesDefaultDirs(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	reg := NewRegistry()
	assert.Contains(t, reg.skillDirs, filepath.Join(home, ".agents/skills"))
	assert.Contains(t, reg.skillDirs, filepath.Join(home, ".goa/skills"))
}

func TestParseFrontMatterDescriptionWithColon(t *testing.T) {
	skill, err := parse([]byte(`---
name: golang
description: Use for Go programming tasks: writing services and tests.
---
# Instructions
Use Go conventions.
`))
	require.NoError(t, err)
	assert.Equal(t, "golang", skill.Name)
	assert.Equal(t, "Use for Go programming tasks: writing services and tests.", skill.Description)
	assert.Contains(t, skill.Body, "# Instructions")
}

func TestToPromptXML(t *testing.T) {
	skills := []*Skill{
		{Name: "pdf-processing", Description: "Extract text from PDFs.", Location: "/path/to/pdf-processing/SKILL.md"},
	}
	xml := ToPromptXML(skills)
	assert.Contains(t, xml, "<available_skills>")
	assert.Contains(t, xml, "<name>pdf-processing</name>")
	assert.Contains(t, xml, "<description>Extract text from PDFs.</description>")
}
