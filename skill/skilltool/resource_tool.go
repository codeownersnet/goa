package skilltool

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/skill"
	"github.com/codeownersnet/goa/tool"
)

type ResourceTool struct {
	Registry *skill.Registry
}

func NewResourceTool(registry *skill.Registry) *ResourceTool {
	return &ResourceTool{Registry: registry}
}

func (t *ResourceTool) Name() string {
	return "read_skill_resource"
}

func (t *ResourceTool) Description() string {
	return "Reads a resource file from an activated skill."
}

func (t *ResourceTool) Process(_ context.Context, args map[string]any) (map[string]any, error) {
	name, ok := args["name"].(string)
	if !ok {
		return nil, fmt.Errorf("read_skill_resource: 'name' argument must be a string")
	}
	relativePath, _ := args["relative_path"].(string)
	data, err := t.Registry.ReadResource(name, relativePath)
	if err != nil {
		return nil, fmt.Errorf("read_skill_resource: %w", err)
	}
	return map[string]any{"content": string(data)}, nil
}

func (t *ResourceTool) Execute(name string, relativePath string) ([]byte, error) {
	return t.Registry.ReadResource(name, relativePath)
}

var _ tool.Tool = (*ResourceTool)(nil)
var _ tool.Declarer = (*ResourceTool)(nil)

func (t *ResourceTool) Declaration() *content.ToolDeclaration {
	return &content.ToolDeclaration{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: schema.Object(map[string]*schema.Schema{
			"name":          schema.String("The skill name"),
			"relative_path": schema.String("The relative path of the resource within the skill"),
		}, "name"),
	}
}
