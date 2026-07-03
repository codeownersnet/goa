package skilltool

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/skill"
	"github.com/codeownersnet/goa/tool"
)

type ScriptTool struct {
	Registry *skill.Registry
}

func NewScriptTool(registry *skill.Registry) *ScriptTool {
	return &ScriptTool{Registry: registry}
}

func (t *ScriptTool) Name() string {
	return "run_skill_script"
}

func (t *ScriptTool) Description() string {
	return "Runs an executable script from an activated skill."
}

func (t *ScriptTool) Process(ctx context.Context, args map[string]any) (map[string]any, error) {
	name, ok := args["name"].(string)
	if !ok {
		return nil, fmt.Errorf("run_skill_script: 'name' argument must be a string")
	}
	scriptPath, ok := args["script_path"].(string)
	if !ok {
		return nil, fmt.Errorf("run_skill_script: 'script_path' argument must be a string")
	}
	output, err := t.Registry.RunScript(ctx, name, scriptPath)
	if err != nil {
		return nil, fmt.Errorf("run_skill_script: %w", err)
	}
	return map[string]any{"output": string(output)}, nil
}

func (t *ScriptTool) Execute(name string, scriptPath string) ([]byte, error) {
	return t.Registry.RunScript(context.Background(), name, scriptPath)
}

var _ tool.Tool = (*ScriptTool)(nil)
var _ tool.Declarer = (*ScriptTool)(nil)

func (t *ScriptTool) Declaration() *content.ToolDeclaration {
	return &content.ToolDeclaration{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: schema.Object(map[string]*schema.Schema{
			"name":        schema.String("The skill name"),
			"script_path": schema.String("The path of the script within the skill"),
		}, "name", "script_path"),
	}
}
