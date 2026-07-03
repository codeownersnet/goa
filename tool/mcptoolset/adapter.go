package mcptoolset

import (
	"context"
	"fmt"
	"strings"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpTool struct {
	name        string
	description string
	declaration *content.ToolDeclaration
	session     *mcp.ClientSession
	serverName  string
	toolName    string
	timeout     int
}

func newMCPTool(serverName string, def *mcp.Tool, session *mcp.ClientSession, timeout int) *mcpTool {
	sanitizedServer := sanitizeName(serverName)
	sanitizedTool := sanitizeName(def.Name)
	prefixedName := sanitizedServer + "_" + sanitizedTool

	desc := def.Description
	if def.Annotations != nil && def.Annotations.Title != "" {
		desc = def.Annotations.Title
	}
	if desc == "" {
		desc = def.Name
	}

	var inputSchema *schema.Schema
	if def.InputSchema != nil {
		inputSchema = convertInputSchema(def.InputSchema)
	} else {
		inputSchema = schema.Object(nil)
	}

	decl := &content.ToolDeclaration{
		Name:        prefixedName,
		Description: desc,
		Parameters:  inputSchema,
	}

	return &mcpTool{
		name:        prefixedName,
		description: desc,
		declaration: decl,
		session:     session,
		serverName:  serverName,
		toolName:    def.Name,
		timeout:     timeout,
	}
}

func (t *mcpTool) Name() string {
	return t.name
}

func (t *mcpTool) Description() string {
	return t.description
}

func (t *mcpTool) Process(ctx context.Context, args map[string]any) (map[string]any, error) {
	params := &mcp.CallToolParams{
		Name:      t.toolName,
		Arguments: args,
	}

	result, err := t.session.CallTool(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("mcp tool %q: %w", t.toolName, err)
	}

	if result.IsError {
		errMsg := extractTextContent(result.Content)
		return nil, fmt.Errorf("mcp tool %q error: %s", t.toolName, errMsg)
	}

	output := make(map[string]any)

	text := extractTextContent(result.Content)
	if text != "" {
		output["result"] = text
	}

	if result.StructuredContent != nil {
		output["structured"] = result.StructuredContent
	}

	if len(output) == 0 {
		return map[string]any{}, nil
	}

	return output, nil
}

func (t *mcpTool) Declaration() *content.ToolDeclaration {
	return t.declaration
}

var _ tool.Tool = (*mcpTool)(nil)
var _ tool.Declarer = (*mcpTool)(nil)

func extractTextContent(contents []mcp.Content) string {
	var parts []string
	for _, c := range contents {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
