package mcptoolset

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (ts *Toolset) Complete(ctx context.Context, req tool.CompletionRequest) (tool.CompletionResult, error) {
	session := ts.currentSession()
	if session == nil {
		return tool.CompletionResult{}, errNotConnected
	}

	params := &mcp.CompleteParams{
		Argument: mcp.CompleteParamsArgument{
			Name:  req.Argument,
			Value: req.Value,
		},
		Ref: &mcp.CompleteReference{
			Type: req.Ref.Type,
			Name: req.Ref.Name,
			URI:  req.Ref.URI,
		},
	}
	if len(req.Arguments) > 0 {
		params.Context = &mcp.CompleteContext{Arguments: req.Arguments}
	}

	result, err := session.Complete(ctx, params)
	if err != nil {
		return tool.CompletionResult{}, fmt.Errorf("mcptoolset %q: complete %q: %w", ts.name, req.Ref.Type, err)
	}

	return tool.CompletionResult{
		Values:  result.Completion.Values,
		Total:   result.Completion.Total,
		HasMore: result.Completion.HasMore,
	}, nil
}

var _ tool.CompletionProvider = (*Toolset)(nil)
