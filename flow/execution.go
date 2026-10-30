package flow

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
)

type ToolScheduling int

const (
	ToolSchedulingParallel ToolScheduling = iota
	ToolSchedulingSequential
)

type ModelGuard func(agent.InvocationContext, *model.ModelRequest) error

type ToolGuard func(tool.Context, tool.Tool, map[string]any) error

type CompletionGuard func(agent.InvocationContext, *session.Event) error

type ContextPolicy interface {
	Apply(agent.InvocationContext, *model.ModelRequest) error
}

type ContextPolicyFunc func(agent.InvocationContext, *model.ModelRequest) error

func (f ContextPolicyFunc) Apply(ctx agent.InvocationContext, req *model.ModelRequest) error {
	return f(ctx, req)
}

type OutputValidator interface {
	Validate(agent.InvocationContext, *session.Event) error
}

type OutputValidatorFunc func(agent.InvocationContext, *session.Event) error

func (f OutputValidatorFunc) Validate(ctx agent.InvocationContext, event *session.Event) error {
	return f(ctx, event)
}

type RunBudget struct {
	ModelTurns        int
	ToolCalls         int
	ToolCallsThisTurn int
	OutputBytes       int
}

type RunLimits struct {
	MaxModelTurns       int
	MaxToolCallsPerTurn int
	MaxToolCalls        int
	MaxPromptBytes      int
	MaxToolResultBytes  int
	MaxOutputBytes      int
	MaxToolDuration     time.Duration
	RunDeadline         time.Duration
	Budget              *RunBudget
}

type RequiredCapabilities struct {
	ToolCall         bool
	StructuredOutput bool
	Reasoning        bool
	Attachment       bool
}

type ExecutionOptions struct {
	ToolScheduling         ToolScheduling
	MaxParallelTools       int
	Limits                 RunLimits
	ContextPolicy          ContextPolicy
	BeforeModelGuards      []ModelGuard
	BeforeToolGuards       []ToolGuard
	BeforeCompletionGuards []CompletionGuard
	OutputValidator        OutputValidator
	MaxValidationRepairs   int
	RequiredCapabilities   RequiredCapabilities
}

func (o ExecutionOptions) validateCapabilities(caps model.ModelCapabilities) error {
	switch {
	case o.RequiredCapabilities.ToolCall && !caps.ToolCall:
		return fmt.Errorf("model does not support required capability %q", "tool_call")
	case o.RequiredCapabilities.StructuredOutput && !caps.StructuredOutput:
		return fmt.Errorf("model does not support required capability %q", "structured_output")
	case o.RequiredCapabilities.Reasoning && !caps.Reasoning:
		return fmt.Errorf("model does not support required capability %q", "reasoning")
	case o.RequiredCapabilities.Attachment && !caps.Attachment:
		return fmt.Errorf("model does not support required capability %q", "attachment")
	}
	return nil
}

func (o ExecutionOptions) budget() *RunBudget {
	if o.Limits.Budget != nil {
		return o.Limits.Budget
	}
	return &RunBudget{}
}

func requestBytes(req *model.ModelRequest) (int, error) {
	data, err := json.Marshal(struct {
		Contents []*content.Content    `json:"contents"`
		Config   *model.GenerateConfig `json:"config,omitempty"`
		Tools    map[string]any        `json:"tools,omitempty"`
	}{
		Contents: req.Contents,
		Config:   req.Config,
		Tools:    req.Tools,
	})
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func responseTextBytes(resp *model.ModelResponse) int {
	if resp.Content == nil {
		return 0
	}
	total := 0
	for _, part := range resp.Content.Parts {
		if part.Text != nil {
			total += len(part.Text.Text)
		}
	}
	return total
}

func toolResultBytes(result map[string]any) (int, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func SlidingWindowContextPolicy(maxBytes int) ContextPolicy {
	return ContextPolicyFunc(func(_ agent.InvocationContext, req *model.ModelRequest) error {
		if maxBytes <= 0 {
			return nil
		}
		original := req.Contents
		start := 0
		for {
			for start < len(original)-1 && original[start].Role == content.RoleTool {
				start++
			}
			req.Contents = original[start:]
			size, err := requestBytes(req)
			if err != nil {
				req.Contents = original
				return err
			}
			if size <= maxBytes {
				return nil
			}
			if start >= len(original)-1 {
				break
			}
			start++
		}
		req.Contents = original
		return fmt.Errorf("context requires at least %d bytes, limit is %d", mustRequestBytes(req), maxBytes)
	})
}

func mustRequestBytes(req *model.ModelRequest) int {
	size, _ := requestBytes(req)
	return size
}
