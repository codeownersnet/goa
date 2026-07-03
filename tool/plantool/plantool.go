package plantool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
)

type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusInProgress StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusSkipped    StepStatus = "skipped"
)

type PlanStep struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Status StepStatus `json:"status"`
	Order  int        `json:"order"`
}

type Plan struct {
	Title       string     `json:"title"`
	Goal        string     `json:"goal"`
	Steps       []PlanStep `json:"steps"`
	CurrentStep string     `json:"current_step,omitempty"`
}

const StateKey = "goa:plan"

func New() ([]tool.Tool, error) {
	create, err := newCreateTool()
	if err != nil {
		return nil, err
	}
	update, err := newUpdateTool()
	if err != nil {
		return nil, err
	}
	get, err := newGetTool()
	if err != nil {
		return nil, err
	}
	show, err := newShowTool()
	if err != nil {
		return nil, err
	}
	return []tool.Tool{create, update, get, show}, nil
}

func getPlan(state session.State) (*Plan, error) {
	raw, err := state.Get(StateKey)
	if err != nil {
		return nil, err
	}
	switch v := raw.(type) {
	case Plan:
		return &v, nil
	case *Plan:
		return v, nil
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("plantool: corrupted plan data: %w", err)
		}
		var plan Plan
		if err := json.Unmarshal(b, &plan); err != nil {
			return nil, fmt.Errorf("plantool: corrupted plan data: %w", err)
		}
		return &plan, nil
	default:
		return nil, fmt.Errorf("plantool: unexpected plan type %T", raw)
	}
}

func (p *Plan) advanceToNextPendingStep() {
	for i := range p.Steps {
		if p.Steps[i].Status == StepStatusPending {
			p.Steps[i].Status = StepStatusInProgress
			p.CurrentStep = p.Steps[i].ID
			return
		}
	}
	p.CurrentStep = ""
}

func (p *Plan) allComplete() bool {
	for _, s := range p.Steps {
		if s.Status != StepStatusCompleted && s.Status != StepStatusSkipped {
			return false
		}
	}
	return true
}

func (p *Plan) nextStepID() string {
	maxID := 0
	for _, s := range p.Steps {
		var id int
		if _, err := fmt.Sscanf(s.ID, "%d", &id); err == nil && id > maxID {
			maxID = id
		}
	}
	return fmt.Sprintf("%d", maxID+1)
}

func renderPlanReminder(plan *Plan) string {
	var b strings.Builder
	b.WriteString("[PLAN REMINDER]\n")
	b.WriteString("You are working on a plan. Follow it step by step.\n\n")
	b.WriteString(fmt.Sprintf("Plan: %s\n", plan.Title))
	b.WriteString(fmt.Sprintf("Goal: %s\n\n", plan.Goal))

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
		b.WriteString(fmt.Sprintf("%s [%s] %s\n", marker, step.ID, step.Title))
	}

	if plan.CurrentStep != "" {
		b.WriteString(fmt.Sprintf("\nCurrent step: %s\n", plan.CurrentStep))
		b.WriteString("Continue working on the current step. Mark it completed with plan_update when done.\n")
	} else if plan.allComplete() {
		b.WriteString("\nAll steps completed!\n")
	}

	return b.String()
}
