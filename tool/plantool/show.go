package plantool

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type showArgs struct{}

type showResult struct {
	DisplayText string `json:"display_text"`
	Exists      bool   `json:"exists"`
	AllComplete bool   `json:"all_complete"`
}

func showHandler(ctx context.Context, _ showArgs) (showResult, error) {
	state := tool.StateFromContext(ctx)
	if state == nil {
		return showResult{Exists: false}, nil
	}

	plan, err := getPlan(state)
	if err != nil {
		return showResult{Exists: false}, nil
	}

	text := fmt.Sprintf("\n========================================\n  PLAN: %s\n  Goal: %s\n========================================\n\n", plan.Title, plan.Goal)
	for _, step := range plan.Steps {
		marker := "○"
		switch step.Status {
		case StepStatusCompleted:
			marker = "✓"
		case StepStatusInProgress:
			marker = "►"
		case StepStatusSkipped:
			marker = "⊘"
		}
		text += fmt.Sprintf("  %s [%s] %s\n", marker, step.ID, step.Title)
	}

	if plan.CurrentStep != "" {
		text += fmt.Sprintf("\n  CURRENT STEP: %s\n", plan.CurrentStep)
	} else if plan.allComplete() {
		text += "\n  ALL STEPS COMPLETED\n"
	}

	text += "\n========================================\n"

	return showResult{
		DisplayText: text,
		Exists:      true,
		AllComplete: plan.allComplete(),
	}, nil
}

func newShowTool() (tool.Tool, error) {
	ft, err := functiontool.New(functiontool.Config{
		Name:        "plan_show",
		Description: "Display the current plan to the user in a readable format. Use before starting work and after finishing to show progress.",
	}, showHandler)
	if err != nil {
		return nil, fmt.Errorf("plantool: %w", err)
	}
	return ft, nil
}
