package mcptoolset

import (
	"context"
	"fmt"
	"strings"

	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (ts *Toolset) Prompts() []tool.PromptInfo {
	return ts.prompts
}

func (ts *Toolset) GetPrompt(ctx context.Context, name string, args map[string]string) (string, error) {
	result, err := ts.session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("mcptoolset %q: get prompt %q: %w", ts.name, name, err)
	}

	var parts []string
	for _, msg := range result.Messages {
		if tc, ok := msg.Content.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}

	return strings.Join(parts, "\n"), nil
}

func (ts *Toolset) discoverPrompts(ctx context.Context) error {
	var prompts []tool.PromptInfo
	for p, err := range ts.session.Prompts(ctx, nil) {
		if err != nil {
			return fmt.Errorf("list prompts: %w", err)
		}
		pi := tool.PromptInfo{
			Name:        p.Name,
			Description: p.Description,
		}
		for _, arg := range p.Arguments {
			pi.Arguments = append(pi.Arguments, tool.PromptArgument{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			})
		}
		prompts = append(prompts, pi)
	}
	ts.prompts = prompts
	return nil
}

var _ tool.PromptProvider = (*Toolset)(nil)
