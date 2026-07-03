package exitlooptool

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type exitLoopArgs struct{}

func exitLoopHandler(ctx context.Context, _ exitLoopArgs) (map[string]any, error) {
	if actions := tool.ActionsFromContext(ctx); actions != nil {
		actions.Escalate = true
		actions.SkipSummarization = true
	}
	return map[string]any{}, nil
}

func New() (tool.Tool, error) {
	ft, err := functiontool.New(functiontool.Config{
		Name:        "exit_loop",
		Description: "Exits the loop.\n\nCall this function only when you are instructed to do so.\n",
	}, exitLoopHandler)
	if err != nil {
		return nil, fmt.Errorf("exitlooptool: %w", err)
	}
	return ft, nil
}
