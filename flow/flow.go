package flow

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/truncate"
)

type BeforeModelCallback func(agent.InvocationContext, *model.ModelRequest) (*model.ModelResponse, error)

type AfterModelCallback func(agent.InvocationContext, *model.ModelResponse, error) (*model.ModelResponse, error)

type OnModelErrorCallback func(agent.InvocationContext, *model.ModelRequest, error) (*model.ModelResponse, error)

type BeforeToolCallback func(tool.Context, tool.Tool, map[string]any) (map[string]any, error)

type AfterToolCallback func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)

type OnToolErrorCallback func(tool.Context, tool.Tool, map[string]any, error) (map[string]any, error)

type RequestProcessor func(agent.InvocationContext, *model.ModelRequest, *Flow) iter.Seq2[*session.Event, error]

type Flow struct {
	Model                 model.Model
	Tools                 []tool.Tool
	Instruction           string
	GenerateConfig        *model.GenerateConfig
	MaxIterations         int
	RequireToolUse        bool
	ToolTimeout           time.Duration
	RequestProcessors     []RequestProcessor
	ResponseProcessors    []func(agent.InvocationContext, *model.ModelRequest, *model.ModelResponse) error
	BeforeModelCallbacks  []BeforeModelCallback
	AfterModelCallbacks   []AfterModelCallback
	OnModelErrorCallbacks []OnModelErrorCallback
	BeforeToolCallbacks   []BeforeToolCallback
	AfterToolCallbacks    []AfterToolCallback
	OnToolErrorCallbacks  []OnToolErrorCallback
}

func (f *Flow) Run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		maxIter := f.MaxIterations
		if maxIter <= 0 {
			maxIter = 25
		}
		detector := &doomLoopDetector{maxConsecutive: 3}
		doomLoops := 0
		usedTools := false
		for i := 0; i < maxIter; i++ {
			var events []*session.Event
			for ev, err := range f.runOneStep(ctx) {
				if err != nil {
					yield(nil, err)
					return
				}
				if ev != nil && eventHasFunctionCalls(ev) {
					usedTools = true
				}
				if !yield(ev, nil) {
					return
				}
				events = append(events, ev)
			}
			var lastEvent *session.Event
			if len(events) > 0 {
				lastEvent = events[len(events)-1]
			}
			if lastEvent == nil || lastEvent.IsFinalResponse() {
				if f.RequireToolUse && !usedTools && lastEvent != nil {
					yield(nil, fmt.Errorf("agent stopped without using tools: text-only response"))
					return
				}
				return
			}
			if lastEvent.Partial {
				yield(nil, fmt.Errorf("incomplete response"))
				return
			}
			if detector.checkIteration(events) {
				doomLoops++
				if doomLoops >= 2 {
					yield(nil, fmt.Errorf("doom loop detected: same tool calls repeated %d times across iterations", detector.maxConsecutive))
					return
				}
				warning := session.NewEvent(ctx.InvocationID())
				warning.Author = "system"
				warning.ModelResponse = &content.Content{
					Role: content.RoleUser,
					Parts: []content.Part{content.NewTextPart(
						"WARNING: You are in a doom loop — you have repeated the same tool calls multiple times without making progress. " +
							"Stop repeating the same actions. Try a different approach. If a tool call succeeded, move on to the next step. " +
							"If a tool call failed, try a different method.",
					)},
				}
				if !yield(warning, nil) {
					return
				}
			}
		}
		yield(nil, fmt.Errorf("max iterations (%d) reached", maxIter))
	}
}

func (f *Flow) runOneStep(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		if f.Model == nil {
			yield(nil, fmt.Errorf("model not configured"))
			return
		}

		req := &model.ModelRequest{
			Model:  f.Model.Name(),
			Config: f.GenerateConfig,
		}

		for ev, err := range f.preprocess(ctx, req) {
			if err != nil {
				yield(nil, err)
				return
			}
			if ev != nil {
				if !yield(ev, nil) {
					return
				}
			}
		}
		if ctx.Ended() {
			return
		}

		for resp, err := range f.callLLM(ctx, req) {
			if err != nil {
				yield(nil, err)
				return
			}
			if err := f.postprocess(ctx, req, resp); err != nil {
				yield(nil, err)
				return
			}
			if resp.Content == nil && resp.FinishReason == "" {
				continue
			}

			event := f.buildEvent(ctx, resp)
			if !yield(event, nil) {
				return
			}

			if resp.Partial {
				continue
			}

			ev, err := f.handleFunctionCalls(ctx, resp)
			if err != nil {
				yield(nil, err)
				return
			}
			if ev != nil {
				if !yield(ev, nil) {
					return
				}
			}
		}
	}
}

func (f *Flow) preprocess(ctx agent.InvocationContext, req *model.ModelRequest) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		contents := ctx.Session().Events()
		var history []*content.Content
		for ev := range contents.All() {
			if ev.ModelResponse != nil {
				history = append(history, ev.ModelResponse)
			}
		}
		if ctx.UserContent() != nil {
			history = append(history, ctx.UserContent())
		}
		req.Contents = history
		req.Config = withSystemInstruction(req.Config, f.Instruction)
		req.Tools = f.buildToolDeclarations()

		for _, processor := range f.RequestProcessors {
			for ev, err := range processor(ctx, req, f) {
				if err != nil {
					yield(nil, err)
					return
				}
				if ev != nil {
					yield(ev, nil)
				}
			}
		}
	}
}

func withSystemInstruction(cfg *model.GenerateConfig, instruction string) *model.GenerateConfig {
	if instruction == "" {
		return cfg
	}
	if cfg == nil {
		return &model.GenerateConfig{SystemInstruction: instruction}
	}
	copied := *cfg
	if copied.SystemInstruction == "" {
		copied.SystemInstruction = instruction
	} else {
		copied.SystemInstruction = instruction + "\n\n" + copied.SystemInstruction
	}
	return &copied
}

func (f *Flow) postprocess(ctx agent.InvocationContext, req *model.ModelRequest, resp *model.ModelResponse) error {
	for _, processor := range f.ResponseProcessors {
		if err := processor(ctx, req, resp); err != nil {
			return err
		}
	}
	return nil
}

func (f *Flow) callLLM(ctx agent.InvocationContext, req *model.ModelRequest) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		for _, cb := range f.BeforeModelCallbacks {
			resp, err := cb(ctx, req)
			if resp != nil || err != nil {
				if !yield(resp, err) {
					return
				}
				return
			}
		}

		caps := f.Model.Capabilities()
		stream := caps.OutputModalities != nil && caps.OutputModalities["streaming"]

		for resp, err := range f.Model.GenerateContent(ctx, req, stream) {
			if err != nil {
				for _, cb := range f.OnModelErrorCallbacks {
					r, e := cb(ctx, req, err)
					if r != nil || e != nil {
						if !yield(r, e) {
							return
						}
						return
					}
				}
				if !yield(nil, err) {
					return
				}
				return
			}

			for _, cb := range f.AfterModelCallbacks {
				r, e := cb(ctx, resp, nil)
				if r != nil || e != nil {
					if !yield(r, e) {
						return
					}
					return
				}
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}

func (f *Flow) handleFunctionCalls(ctx agent.InvocationContext, resp *model.ModelResponse) (*session.Event, error) {
	if resp.Content == nil {
		return nil, nil
	}

	var calls []*content.FunctionCall
	for _, part := range resp.Content.Parts {
		if part.FunctionCall != nil {
			calls = append(calls, part.FunctionCall)
		}
	}
	if len(calls) == 0 {
		return nil, nil
	}

	results := make([]content.Part, len(calls))
	actionsList := make([]*tool.EventActions, len(calls))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var fatalErr error

	for i, call := range calls {
		actionsList[i] = &tool.EventActions{}
		wg.Add(1)
		go func(idx int, c *content.FunctionCall) {
			defer wg.Done()
			toolActions := &tool.EventActions{}
			result, err := f.executeTool(ctx, c, toolActions)
			var part content.Part
			if err != nil {
				result, isInvalid := f.handleToolError(c, err)
				if isInvalid {
					part = content.NewFunctionResponsePart(c.ID, c.Name, result, true)
				} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					part = content.NewFunctionResponsePart(c.ID, c.Name, map[string]any{"error": err.Error()}, true)
					mu.Lock()
					if fatalErr == nil {
						fatalErr = err
					}
					mu.Unlock()
				} else {
					part = content.NewFunctionResponsePart(c.ID, c.Name, map[string]any{"error": err.Error()}, true)
				}
			} else {
				result = truncateResult(result)
				part = content.NewFunctionResponsePart(c.ID, c.Name, result, false)
			}
			mu.Lock()
			results[idx] = part
			actionsList[idx] = toolActions
			mu.Unlock()
		}(i, call)
	}
	wg.Wait()

	if fatalErr != nil {
		return nil, fatalErr
	}

	var mergedActions session.EventActions
	for _, actions := range actionsList {
		if actions.Escalate {
			mergedActions.Escalate = true
		}
		if actions.SkipSummarization {
			mergedActions.SkipSummarization = true
		}
		if actions.TransferToAgent != "" {
			mergedActions.TransferToAgent = actions.TransferToAgent
		}
		if len(actions.StateDelta) > 0 {
			if mergedActions.StateDelta == nil {
				mergedActions.StateDelta = make(map[string]any)
			}
			for k, v := range actions.StateDelta {
				mergedActions.StateDelta[k] = v
			}
		}
	}

	event := session.NewEvent(ctx.InvocationID())
	event.Author = ctx.Agent().Name()
	event.Branch = ctx.Branch()
	event.ModelResponse = &content.Content{
		Role:  content.RoleTool,
		Parts: results,
	}
	event.Actions = mergedActions
	return event, nil
}

func (f *Flow) handleToolError(call *content.FunctionCall, err error) (map[string]any, bool) {
	if strings.HasSuffix(err.Error(), "not found") && strings.Contains(err.Error(), "tool") {
		var names []string
		for _, t := range f.Tools {
			names = append(names, t.Name())
		}
		return map[string]any{
			"error":           fmt.Sprintf("Unknown tool %q. Available tools: %s", call.Name, strings.Join(names, ", ")),
			"available_tools": names,
		}, true
	}
	return nil, false
}

func (f *Flow) executeTool(ctx agent.InvocationContext, call *content.FunctionCall, actions *tool.EventActions) (map[string]any, error) {
	t := f.resolveTool(call.Name)
	if t == nil {
		return nil, fmt.Errorf("tool %q not found", call.Name)
	}

	timeout := f.ToolTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	timeoutCtx := ctx.WithContext(toolCtx)

	flowCtx := newToolContext(timeoutCtx, call.ID, actions)

	args := call.Args
	for _, cb := range f.BeforeToolCallbacks {
		r, err := cb(flowCtx, t, args)
		if r != nil || err != nil {
			return r, err
		}
		args = r
	}

	processCtx := tool.ContextWithActions(flowCtx, actions)
	processCtx = tool.ContextWithState(processCtx, ctx.Session().State())
	result, err := t.Process(processCtx, args)

	for _, cb := range f.AfterToolCallbacks {
		r, e := cb(flowCtx, t, args, result, err)
		if r != nil || e != nil {
			result = r
			err = e
			break
		}
	}

	if err != nil {
		for _, cb := range f.OnToolErrorCallbacks {
			r, e := cb(flowCtx, t, args, err)
			if r != nil || e != nil {
				return r, e
			}
		}
		return nil, err
	}

	return result, nil
}

func (f *Flow) resolveTool(name string) tool.Tool {
	lower := strings.ToLower(name)
	for _, t := range f.Tools {
		toolName := t.Name()
		if toolName == name || strings.ToLower(toolName) == lower {
			return t
		}
	}
	return nil
}

type doomLoopDetector struct {
	mu             sync.Mutex
	maxConsecutive int
	iterSignatures []string
}

func (d *doomLoopDetector) checkIteration(events []*session.Event) bool {
	var calls []string
	for _, ev := range events {
		if ev == nil || ev.ModelResponse == nil {
			continue
		}
		for _, part := range ev.ModelResponse.Parts {
			if part.FunctionCall != nil {
				calls = append(calls, part.FunctionCall.Name+"("+fmt.Sprintf("%v", part.FunctionCall.Args)+")")
			}
		}
	}
	if len(calls) == 0 {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	sig := strings.Join(calls, "||")
	d.iterSignatures = append(d.iterSignatures, sig)
	if len(d.iterSignatures) > d.maxConsecutive {
		copied := make([]string, d.maxConsecutive)
		copy(copied, d.iterSignatures[len(d.iterSignatures)-d.maxConsecutive:])
		d.iterSignatures = copied
	}

	if len(d.iterSignatures) < d.maxConsecutive {
		return false
	}

	first := d.iterSignatures[0]
	for i := 1; i < len(d.iterSignatures); i++ {
		if d.iterSignatures[i] != first {
			return false
		}
	}
	d.iterSignatures = d.iterSignatures[:0]
	return true
}

type flowToolContext struct {
	agent.InvocationContext
	callID  string
	actions *tool.EventActions
}

func newToolContext(ctx agent.InvocationContext, callID string, actions *tool.EventActions) *flowToolContext {
	return &flowToolContext{InvocationContext: ctx, callID: callID, actions: actions}
}

func (c *flowToolContext) FunctionCallID() string {
	return c.callID
}

func (c *flowToolContext) Actions() *tool.EventActions {
	return c.actions
}

func (c *flowToolContext) AgentName() string {
	return c.InvocationContext.Agent().Name()
}

func (c *flowToolContext) AppName() string {
	return c.InvocationContext.Session().AppName()
}

func (c *flowToolContext) SessionID() string {
	return c.InvocationContext.Session().ID()
}

func (c *flowToolContext) UserID() string {
	return c.InvocationContext.Session().UserID()
}

func (c *flowToolContext) State() session.State {
	return c.InvocationContext.Session().State()
}

func (c *flowToolContext) Artifacts() artifact.Service {
	return c.InvocationContext.ArtifactService()
}

var _ tool.Context = (*flowToolContext)(nil)

func (f *Flow) buildEvent(ctx agent.InvocationContext, resp *model.ModelResponse) *session.Event {
	event := session.NewEvent(ctx.InvocationID())
	event.Author = ctx.Agent().Name()
	event.Branch = ctx.Branch()
	event.Partial = resp.Partial
	event.Usage = resp.Usage
	event.FinishReason = resp.FinishReason
	if resp.Content != nil {
		event.ModelResponse = resp.Content
	}
	return event
}

func (f *Flow) buildToolDeclarations() map[string]any {
	declarations := make(map[string]any)
	for _, t := range f.Tools {
		if d, ok := t.(tool.Declarer); ok {
			declarations[t.Name()] = *d.Declaration()
		} else {
			declarations[t.Name()] = content.ToolDeclaration{
				Name:        t.Name(),
				Description: t.Description(),
			}
		}
	}
	return declarations
}

func eventHasFunctionCalls(e *session.Event) bool {
	if e.ModelResponse == nil {
		return false
	}
	for _, part := range e.ModelResponse.Parts {
		if part.FunctionCall != nil {
			return true
		}
	}
	return false
}

func truncateResult(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}
	truncated := false
	for k, v := range result {
		if s, ok := v.(string); ok {
			t := truncate.Truncate(s, truncate.Options{})
			if t.Truncated {
				result[k] = t.Content
				truncated = true
			}
		}
	}
	if truncated {
		result["_output_truncated"] = true
	}
	return result
}
