package plantool

import (
	"context"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type getArgs struct{}

type getResult struct {
	Plan        Plan `json:"plan"`
	Exists      bool `json:"exists"`
	AllComplete bool `json:"all_complete"`
}

func getHandler(ctx context.Context, _ getArgs) (getResult, error) {
	state := tool.StateFromContext(ctx)
	if state == nil {
		return getResult{Exists: false}, nil
	}

	plan, err := getPlan(state)
	if err != nil {
		return getResult{Exists: false}, nil
	}

	return getResult{
		Plan:        *plan,
		Exists:      true,
		AllComplete: plan.allComplete(),
	}, nil
}

func newGetTool() (tool.Tool, error) {
	ft, err := functiontool.New(functiontool.Config{
		Name:        "plan_get",
		Description: "Get the current execution plan. Returns exists=false if no plan has been created.",
	}, getHandler)
	if err != nil {
		return nil, err
	}
	return ft, nil
}
