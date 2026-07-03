package skilltool

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/skill"
	"github.com/codeownersnet/goa/tool"
)

type ActivateTool struct {
	Registry *skill.Registry
}

func NewActivateTool(registry *skill.Registry) *ActivateTool {
	return &ActivateTool{Registry: registry}
}

func (t *ActivateTool) Name() string {
	return "activate_skill"
}

func (t *ActivateTool) Description() string {
	return "Activates a skill by name, making its instructions and tools available."
}

func (t *ActivateTool) Process(_ context.Context, args map[string]any) (map[string]any, error) {
	name, ok := args["name"].(string)
	if !ok {
		return nil, fmt.Errorf("activate_skill: 'name' argument must be a string")
	}
	s, err := t.Registry.Activate(name)
	if err != nil {
		return nil, fmt.Errorf("activate_skill: %w", err)
	}
	return map[string]any{"name": s.Name, "description": s.Description, "instructions": s.Body}, nil
}

func (t *ActivateTool) Execute(name string) (*skill.Skill, error) {
	return t.Registry.Activate(name)
}

var _ tool.Tool = (*ActivateTool)(nil)
var _ tool.Declarer = (*ActivateTool)(nil)

func (t *ActivateTool) Declaration() *content.ToolDeclaration {
	return &content.ToolDeclaration{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: schema.Object(map[string]*schema.Schema{
			"name": schema.String("The skill name to activate"),
		}, "name"),
	}
}
