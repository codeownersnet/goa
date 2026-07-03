package plantool

import (
	"context"
	"fmt"
	"sort"

	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type updateArgs struct {
	StepID      string   `json:"step_id"`
	Status      string   `json:"status"`
	AddSteps    []string `json:"add_steps,omitempty"`
	InsertAfter string   `json:"insert_after,omitempty"`
}

func updateHandler(ctx context.Context, args updateArgs) (map[string]any, error) {
	state := tool.StateFromContext(ctx)
	actions := tool.ActionsFromContext(ctx)

	if state == nil {
		return nil, fmt.Errorf("plan_update: session state not available")
	}

	plan, err := getPlan(state)
	if err != nil {
		return nil, fmt.Errorf("plan_update: no plan exists, create one first with plan_create")
	}

	if args.StepID != "" {
		newStatus := StepStatus(args.Status)
		found := false
		for i, step := range plan.Steps {
			if step.ID == args.StepID {
				step.Status = newStatus
				plan.Steps[i] = step
				found = true

				if newStatus == StepStatusCompleted || newStatus == StepStatusSkipped {
					if plan.CurrentStep == step.ID {
						plan.advanceToNextPendingStep()
					}
				}
				if newStatus == StepStatusInProgress {
					plan.CurrentStep = step.ID
				}
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("plan_update: step %q not found", args.StepID)
		}
	}

	if len(args.AddSteps) > 0 {
		maxOrder := 0
		for _, s := range plan.Steps {
			if s.Order > maxOrder {
				maxOrder = s.Order
			}
		}
		insertAfterOrder := maxOrder
		if args.InsertAfter != "" {
			for _, s := range plan.Steps {
				if s.ID == args.InsertAfter {
					insertAfterOrder = s.Order
					break
				}
			}
		}
		for i := range plan.Steps {
			if plan.Steps[i].Order > insertAfterOrder {
				plan.Steps[i].Order += len(args.AddSteps)
			}
		}
		for i, stepTitle := range args.AddSteps {
			plan.Steps = append(plan.Steps, PlanStep{
				ID:     plan.nextStepID(),
				Title:  stepTitle,
				Status: StepStatusPending,
				Order:  insertAfterOrder + i + 1,
			})
		}
		sort.Slice(plan.Steps, func(i, j int) bool {
			return plan.Steps[i].Order < plan.Steps[j].Order
		})
	}

	if actions != nil {
		if actions.StateDelta == nil {
			actions.StateDelta = make(map[string]any)
		}
		actions.StateDelta[StateKey] = *plan
	}

	return map[string]any{
		"plan":         *plan,
		"current_step": plan.CurrentStep,
	}, nil
}

func newUpdateTool() (tool.Tool, error) {
	ft, err := functiontool.New(functiontool.Config{
		Name:        "plan_update",
		Description: "Update a step's status or add new steps to the plan. When a step is completed, the next pending step is automatically started. Use add_steps with insert_after to insert plain string steps after a specific step ID.",
		InputSchema: schema.Object(map[string]*schema.Schema{
			"step_id": schema.String("ID of the step to update. Required when changing status."),
			"status": {
				Type:        "string",
				Description: "New status for the step.",
				Enum:        []any{string(StepStatusPending), string(StepStatusInProgress), string(StepStatusCompleted), string(StepStatusSkipped)},
			},
			"add_steps":    schema.Array(schema.String("Plain step description."), "Additional plain string steps to add."),
			"insert_after": schema.String("Optional step ID to insert new steps after. If omitted, steps are appended."),
		}),
	}, updateHandler)
	if err != nil {
		return nil, fmt.Errorf("plantool: %w", err)
	}
	return ft, nil
}
