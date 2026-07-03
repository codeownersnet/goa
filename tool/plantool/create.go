package plantool

import (
	"context"
	"fmt"
	"strconv"

	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type createArgs struct {
	Title string   `json:"title"`
	Goal  string   `json:"goal"`
	Steps []string `json:"steps"`
}

type createResult struct {
	Plan        Plan `json:"plan"`
	Overwritten bool `json:"overwritten"`
}

func createHandler(ctx context.Context, args createArgs) (createResult, error) {
	state := tool.StateFromContext(ctx)
	actions := tool.ActionsFromContext(ctx)

	if len(args.Steps) == 0 {
		return createResult{}, fmt.Errorf("plan_create: at least one step is required")
	}

	plan := Plan{
		Title: args.Title,
		Goal:  args.Goal,
	}

	for i, stepTitle := range args.Steps {
		id := strconv.Itoa(i + 1)
		s := PlanStep{
			ID:    id,
			Title: stepTitle,
			Order: i + 1,
		}
		if i == 0 {
			s.Status = StepStatusInProgress
			plan.CurrentStep = id
		} else {
			s.Status = StepStatusPending
		}
		plan.Steps = append(plan.Steps, s)
	}

	var overwritten bool
	if state != nil {
		if _, err := state.Get(StateKey); err == nil {
			overwritten = true
		}
	}

	if actions != nil {
		if actions.StateDelta == nil {
			actions.StateDelta = make(map[string]any)
		}
		actions.StateDelta[StateKey] = plan
	}

	return createResult{Plan: plan, Overwritten: overwritten}, nil
}

func newCreateTool() (tool.Tool, error) {
	ft, err := functiontool.New(functiontool.Config{
		Name:        "plan_create",
		Description: "Create a structured execution plan with ordered steps. Overwrites any existing plan. The first step is automatically marked as in_progress. Steps must be an array of plain strings.",
		InputSchema: schema.Object(map[string]*schema.Schema{
			"title": schema.String("Short plan title."),
			"goal":  schema.String("Goal the plan is intended to accomplish."),
			"steps": schema.Array(schema.String("Plain step description."), "Ordered list of plain string step descriptions."),
		}, "title", "goal", "steps"),
	}, createHandler)
	if err != nil {
		return nil, fmt.Errorf("plantool: %w", err)
	}
	return ft, nil
}
