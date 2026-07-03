package plantool

import (
	"context"
	"iter"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
)

type testState struct {
	mu   sync.RWMutex
	data map[string]any
}

func newTestState() *testState {
	return &testState{data: make(map[string]any)}
}

func (s *testState) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	if !ok {
		return nil, session.ErrStateKeyNotExist
	}
	return val, nil
}

func (s *testState) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *testState) All() iter.Seq2[string, any] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return func(yield func(string, any) bool) {
		for k, v := range s.data {
			if !yield(k, v) {
				return
			}
		}
	}
}

func ctxWithStateAndActions(state *testState, actions *tool.EventActions) context.Context {
	ctx := context.Background()
	ctx = tool.ContextWithState(ctx, state)
	ctx = tool.ContextWithActions(ctx, actions)
	return ctx
}

func TestNewCreatesAllTools(t *testing.T) {
	tools, err := New()
	require.NoError(t, err)
	require.Len(t, tools, 4)
	assert.Equal(t, "plan_create", tools[0].Name())
	assert.Equal(t, "plan_update", tools[1].Name())
	assert.Equal(t, "plan_get", tools[2].Name())
	assert.Equal(t, "plan_show", tools[3].Name())
}

func TestNewBundle(t *testing.T) {
	bundle, err := NewBundle()
	require.NoError(t, err)
	require.Len(t, bundle.Tools, 4)
	assert.NotNil(t, bundle.Reminder)
}

func TestPlanCreate(t *testing.T) {
	t.Run("creates plan with steps", func(t *testing.T) {
		state := newTestState()
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, err := New()
		require.NoError(t, err)

		result, err := tools[0].Process(ctx, map[string]any{
			"title": "Refactor auth",
			"goal":  "Split auth into packages",
			"steps": []any{"Extract types", "Move handlers", "Update imports"},
		})
		require.NoError(t, err)

		assert.Equal(t, false, result["overwritten"])

		plan, ok := actions.StateDelta[StateKey]
		require.True(t, ok)
		p := plan.(Plan)
		assert.Equal(t, "Refactor auth", p.Title)
		assert.Equal(t, "Split auth into packages", p.Goal)
		require.Len(t, p.Steps, 3)
		assert.Equal(t, StepStatusInProgress, p.Steps[0].Status)
		assert.Equal(t, StepStatusPending, p.Steps[1].Status)
		assert.Equal(t, StepStatusPending, p.Steps[2].Status)
		assert.Equal(t, "1", p.CurrentStep)
	})

	t.Run("rejects empty steps", func(t *testing.T) {
		tools, _ := New()
		_, err := tools[0].Process(context.Background(), map[string]any{
			"title": "Empty plan",
			"goal":  "Nothing",
			"steps": []any{},
		})
		assert.Error(t, err)
	})

	t.Run("overwrites existing plan", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{Title: "old", Goal: "old goal", Steps: []PlanStep{{ID: "1", Title: "old step", Status: StepStatusCompleted, Order: 1}}})
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		result, err := tools[0].Process(ctx, map[string]any{
			"title": "new plan",
			"goal":  "new goal",
			"steps": []any{"new step"},
		})
		require.NoError(t, err)
		assert.Equal(t, true, result["overwritten"])

		plan := actions.StateDelta[StateKey].(Plan)
		assert.Equal(t, "new plan", plan.Title)
	})
}

func TestPlanUpdate(t *testing.T) {
	t.Run("marks step completed and advances", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title:       "Test",
			Goal:        "Test goal",
			CurrentStep: "1",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusInProgress, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusPending, Order: 2},
				{ID: "3", Title: "Step 3", Status: StepStatusPending, Order: 3},
			},
		})
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		result, err := tools[1].Process(ctx, map[string]any{
			"step_id": "1",
			"status":  "completed",
		})
		require.NoError(t, err)

		plan := actions.StateDelta[StateKey].(Plan)
		assert.Equal(t, StepStatusCompleted, plan.Steps[0].Status)
		assert.Equal(t, StepStatusInProgress, plan.Steps[1].Status)
		assert.Equal(t, "2", plan.CurrentStep)
		assert.Equal(t, "2", result["current_step"])
	})

	t.Run("marks last step completed clears current", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title:       "Test",
			Goal:        "Test goal",
			CurrentStep: "2",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusInProgress, Order: 2},
			},
		})
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		result, err := tools[1].Process(ctx, map[string]any{
			"step_id": "2",
			"status":  "completed",
		})
		require.NoError(t, err)

		plan := actions.StateDelta[StateKey].(Plan)
		assert.Equal(t, "", plan.CurrentStep)
		assert.Equal(t, "", result["current_step"])
	})

	t.Run("marks step skipped", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title:       "Test",
			Goal:        "Test goal",
			CurrentStep: "1",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusInProgress, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusPending, Order: 2},
			},
		})
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		_, err := tools[1].Process(ctx, map[string]any{
			"step_id": "1",
			"status":  "skipped",
		})
		require.NoError(t, err)

		plan := actions.StateDelta[StateKey].(Plan)
		assert.Equal(t, StepStatusSkipped, plan.Steps[0].Status)
		assert.Equal(t, StepStatusInProgress, plan.Steps[1].Status)
		assert.Equal(t, "2", plan.CurrentStep)
	})

	t.Run("adds steps at end", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title:       "Test",
			Goal:        "Test goal",
			CurrentStep: "1",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusInProgress, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusPending, Order: 2},
			},
		})
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		_, err := tools[1].Process(ctx, map[string]any{
			"add_steps": []any{"New step 1", "New step 2"},
		})
		require.NoError(t, err)

		plan := actions.StateDelta[StateKey].(Plan)
		require.Len(t, plan.Steps, 4)
		assert.Equal(t, "New step 1", plan.Steps[2].Title)
		assert.Equal(t, "New step 2", plan.Steps[3].Title)
		assert.Equal(t, StepStatusPending, plan.Steps[2].Status)
		assert.Equal(t, StepStatusPending, plan.Steps[3].Status)
	})

	t.Run("adds steps after specific step", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title:       "Test",
			Goal:        "Test goal",
			CurrentStep: "1",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusInProgress, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusPending, Order: 2},
				{ID: "3", Title: "Step 3", Status: StepStatusPending, Order: 3},
			},
		})
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		_, err := tools[1].Process(ctx, map[string]any{
			"add_steps":    []any{"Inserted step"},
			"insert_after": "1",
		})
		require.NoError(t, err)

		plan := actions.StateDelta[StateKey].(Plan)
		require.Len(t, plan.Steps, 4)
		assert.Equal(t, "Inserted step", plan.Steps[1].Title)
		assert.Equal(t, 2, plan.Steps[1].Order)
		assert.Equal(t, 3, plan.Steps[2].Order)
		assert.Equal(t, 4, plan.Steps[3].Order)
	})

	t.Run("error when no plan exists", func(t *testing.T) {
		state := newTestState()
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		_, err := tools[1].Process(ctx, map[string]any{
			"step_id": "1",
			"status":  "completed",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no plan exists")
	})

	t.Run("error for nonexistent step", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title: "Test",
			Goal:  "Test goal",
			Steps: []PlanStep{{ID: "1", Title: "Step 1", Status: StepStatusPending, Order: 1}},
		})
		actions := &tool.EventActions{}
		ctx := ctxWithStateAndActions(state, actions)

		tools, _ := New()
		_, err := tools[1].Process(ctx, map[string]any{
			"step_id": "99",
			"status":  "completed",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestPlanGet(t *testing.T) {
	t.Run("returns existing plan", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title: "Test",
			Goal:  "Test goal",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusPending, Order: 2},
			},
		})
		ctx := tool.ContextWithState(context.Background(), state)

		tools, _ := New()
		result, err := tools[2].Process(ctx, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, true, result["exists"])
		assert.Equal(t, false, result["all_complete"])
	})

	t.Run("returns exists false when no plan", func(t *testing.T) {
		state := newTestState()
		ctx := tool.ContextWithState(context.Background(), state)

		tools, _ := New()
		result, err := tools[2].Process(ctx, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, false, result["exists"])
	})

	t.Run("returns all_complete true when done", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title: "Test",
			Goal:  "Test goal",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
			},
		})
		ctx := tool.ContextWithState(context.Background(), state)

		tools, _ := New()
		result, err := tools[2].Process(ctx, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, true, result["exists"])
		assert.Equal(t, true, result["all_complete"])
	})
}

func TestGetPlanFromDeserializedMap(t *testing.T) {
	state := newTestState()
	state.Set(StateKey, map[string]any{
		"title": "Test",
		"goal":  "Test goal",
		"steps": []any{
			map[string]any{
				"id":     "1",
				"title":  "Step 1",
				"status": "pending",
				"order":  1,
			},
		},
	})

	plan, err := getPlan(state)
	require.NoError(t, err)
	assert.Equal(t, "Test", plan.Title)
	assert.Len(t, plan.Steps, 1)
	assert.Equal(t, StepStatusPending, plan.Steps[0].Status)
}

func TestRenderPlanReminder(t *testing.T) {
	plan := &Plan{
		Title:       "Refactor auth",
		Goal:        "Split auth into packages",
		CurrentStep: "2",
		Steps: []PlanStep{
			{ID: "1", Title: "Extract types", Status: StepStatusCompleted, Order: 1},
			{ID: "2", Title: "Move handlers", Status: StepStatusInProgress, Order: 2},
			{ID: "3", Title: "Update imports", Status: StepStatusPending, Order: 3},
		},
	}

	reminder := renderPlanReminder(plan)
	assert.Contains(t, reminder, "[PLAN REMINDER]")
	assert.Contains(t, reminder, "Plan: Refactor auth")
	assert.Contains(t, reminder, "✓ [1] Extract types")
	assert.Contains(t, reminder, "► [2] Move handlers")
	assert.Contains(t, reminder, "○ [3] Update imports")
	assert.Contains(t, reminder, "Current step: 2")
	assert.Contains(t, reminder, "plan_update")
}

func TestRenderPlanReminderAllComplete(t *testing.T) {
	plan := &Plan{
		Title: "Done",
		Goal:  "Everything is done",
		Steps: []PlanStep{
			{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
		},
	}

	reminder := renderPlanReminder(plan)
	assert.Contains(t, reminder, "All steps completed!")
}

func TestAdvanceToNextPendingStep(t *testing.T) {
	t.Run("advances to next pending", func(t *testing.T) {
		p := &Plan{
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusPending, Order: 2},
				{ID: "3", Title: "Step 3", Status: StepStatusPending, Order: 3},
			},
		}
		p.advanceToNextPendingStep()
		assert.Equal(t, "2", p.CurrentStep)
		assert.Equal(t, StepStatusInProgress, p.Steps[1].Status)
	})

	t.Run("clears current when all done", func(t *testing.T) {
		p := &Plan{
			CurrentStep: "1",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
			},
		}
		p.advanceToNextPendingStep()
		assert.Equal(t, "", p.CurrentStep)
	})
}

func TestPlanShow(t *testing.T) {
	t.Run("returns display when plan exists", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title:       "Test Plan",
			Goal:        "Test goal",
			CurrentStep: "2",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
				{ID: "2", Title: "Step 2", Status: StepStatusInProgress, Order: 2},
			},
		})
		ctx := tool.ContextWithState(context.Background(), state)

		tools, _ := New()
		result, err := tools[3].Process(ctx, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, true, result["exists"])
		assert.Equal(t, false, result["all_complete"])
		display, ok := result["display_text"].(string)
		assert.True(t, ok)
		assert.Contains(t, display, "Test Plan")
		assert.Contains(t, display, "Step 1")
		assert.Contains(t, display, "Step 2")
		assert.Contains(t, display, "CURRENT STEP")
	})

	t.Run("returns exists false when no plan", func(t *testing.T) {
		state := newTestState()
		ctx := tool.ContextWithState(context.Background(), state)

		tools, _ := New()
		result, err := tools[3].Process(ctx, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, false, result["exists"])
		assert.Equal(t, "", result["display_text"])
	})

	t.Run("shows all complete when done", func(t *testing.T) {
		state := newTestState()
		state.Set(StateKey, Plan{
			Title: "Done Plan",
			Goal:  "All done",
			Steps: []PlanStep{
				{ID: "1", Title: "Step 1", Status: StepStatusCompleted, Order: 1},
			},
		})
		ctx := tool.ContextWithState(context.Background(), state)

		tools, _ := New()
		result, err := tools[3].Process(ctx, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, true, result["exists"])
		assert.Equal(t, true, result["all_complete"])
		display := result["display_text"].(string)
		assert.Contains(t, display, "ALL STEPS COMPLETED")
	})
}
