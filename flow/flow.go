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
	Toolsets              []tool.Toolset
	Instruction           string
	GenerateConfig        *model.GenerateConfig
	MaxIterations         int
	RequireToolUse        bool
	ToolTimeout           time.Duration
	ResultPolicy          ResultPolicy
	ExecutionOptions      ExecutionOptions
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
		if f.Model == nil {
			yield(nil, fmt.Errorf("model not configured"))
			return
		}

		opts := f.ExecutionOptions
		if err := opts.validateCapabilities(f.Model.Capabilities()); err != nil {
			yield(nil, err)
			return
		}

		runCtx := ctx
		if opts.Limits.RunDeadline > 0 {
			deadlineCtx, cancel := context.WithTimeout(ctx, opts.Limits.RunDeadline)
			defer cancel()
			runCtx = ctx.WithContext(deadlineCtx)
		}

		budget := opts.budget()
		maxIter := f.MaxIterations
		if maxIter <= 0 {
			maxIter = 25
		}
		detector := &doomLoopDetector{maxConsecutive: 3}
		doomLoops := 0
		validationRepairs := 0
		usedTools := false

		for i := 0; i < maxIter; i++ {
			if opts.Limits.MaxModelTurns > 0 && budget.ModelTurns >= opts.Limits.MaxModelTurns {
				yield(nil, fmt.Errorf("run exceeded %d model turns", opts.Limits.MaxModelTurns))
				return
			}
			budget.ModelTurns++
			budget.ToolCallsThisTurn = 0

			var events []*session.Event
			for ev, err := range f.runOneStep(runCtx, budget) {
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

				if lastEvent != nil && !lastEvent.Partial && opts.OutputValidator != nil {
					if err := opts.OutputValidator.Validate(runCtx, lastEvent); err != nil {
						if opts.MaxValidationRepairs > 0 && validationRepairs < opts.MaxValidationRepairs {
							validationRepairs++
							repair := session.NewEvent(runCtx.InvocationID())
							repair.Author = "system"
							repair.ModelResponse = &content.Content{
								Role: content.RoleUser,
								Parts: []content.Part{content.NewTextPart(
									fmt.Sprintf("The previous response failed validation: %v. Provide a corrected response.", err),
								)},
							}
							if !yield(repair, nil) {
								return
							}
							continue
						}
						yield(nil, fmt.Errorf("output validation failed: %w", err))
						return
					}
				}

				if lastEvent != nil {
					for _, guard := range opts.BeforeCompletionGuards {
						if err := guard(runCtx, lastEvent); err != nil {
							yield(nil, tool.NewDeniedError(fmt.Errorf("completion guard rejected response: %w", err)))
							return
						}
					}
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
				warning := session.NewEvent(runCtx.InvocationID())
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

func (f *Flow) runOneStep(ctx agent.InvocationContext, budget *RunBudget) iter.Seq2[*session.Event, error] {
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

		opts := f.ExecutionOptions
		if opts.ContextPolicy != nil {
			if err := opts.ContextPolicy.Apply(ctx, req); err != nil {
				yield(nil, fmt.Errorf("context policy rejected request: %w", err))
				return
			}
		}
		if opts.Limits.MaxPromptBytes > 0 {
			size, err := requestBytes(req)
			if err != nil {
				yield(nil, fmt.Errorf("measure prompt: %w", err))
				return
			}
			if size > opts.Limits.MaxPromptBytes {
				yield(nil, fmt.Errorf("prompt exceeded %d bytes", opts.Limits.MaxPromptBytes))
				return
			}
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

			if !resp.Partial && opts.Limits.MaxOutputBytes > 0 {
				budget.OutputBytes += responseTextBytes(resp)
				if budget.OutputBytes > opts.Limits.MaxOutputBytes {
					yield(nil, fmt.Errorf("output exceeded %d bytes", opts.Limits.MaxOutputBytes))
					return
				}
			}

			event := f.buildEvent(ctx, resp)
			if !yield(event, nil) {
				return
			}

			if resp.Partial {
				continue
			}

			ev, err := f.handleFunctionCalls(ctx, resp, budget)
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
		for _, guard := range f.ExecutionOptions.BeforeModelGuards {
			if err := guard(ctx, req); err != nil {
				yield(nil, tool.NewDeniedError(fmt.Errorf("model guard rejected request: %w", err)))
				return
			}
		}
		for _, cb := range f.BeforeModelCallbacks {
			resp, err := cb(ctx, req)
			if resp != nil || err != nil {
				if !yield(resp, err) {
					return
				}
				return
			}
		}

		rc := ctx.RunConfig()
		stream := rc != nil && rc.StreamingMode == agent.StreamingModeSSE

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

func (f *Flow) handleFunctionCalls(ctx agent.InvocationContext, resp *model.ModelResponse, budget *RunBudget) (*session.Event, error) {
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

	opts := f.ExecutionOptions
	if opts.Limits.MaxToolCallsPerTurn > 0 && len(calls) > opts.Limits.MaxToolCallsPerTurn {
		return nil, fmt.Errorf("turn exceeded %d tool calls", opts.Limits.MaxToolCallsPerTurn)
	}
	if opts.Limits.MaxToolCalls > 0 && budget.ToolCalls+len(calls) > opts.Limits.MaxToolCalls {
		return nil, fmt.Errorf("run exceeded %d tool calls", opts.Limits.MaxToolCalls)
	}
	budget.ToolCalls += len(calls)
	budget.ToolCallsThisTurn += len(calls)

	results := make([]content.Part, len(calls))
	actionsList := make([]*tool.EventActions, len(calls))
	var mu sync.Mutex
	var fatalErr error
	setFatal := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if fatalErr == nil || (tool.IsTerminal(err) && !tool.IsTerminal(fatalErr)) {
			fatalErr = err
		}
	}

	runOne := func(ctx agent.InvocationContext, idx int, c *content.FunctionCall) {
		toolActions := &tool.EventActions{}
		result, err := f.executeTool(ctx, c, toolActions)
		var part content.Part
		if err != nil {
			if tool.IsTerminal(err) {
				setFatal(err)
				return
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				part = content.NewFunctionResponsePart(c.ID, c.Name, map[string]any{"error": err.Error()}, true)
				setFatal(err)
			} else {
				result, isInvalid := f.handleToolError(c, err)
				if isInvalid {
					part = content.NewFunctionResponsePart(c.ID, c.Name, result, true)
				} else {
					part = content.NewFunctionResponsePart(c.ID, c.Name, map[string]any{"error": err.Error()}, true)
				}
			}
		} else {
			part = content.NewFunctionResponsePart(c.ID, c.Name, result, false)
		}
		mu.Lock()
		results[idx] = part
		actionsList[idx] = toolActions
		mu.Unlock()
	}

	if opts.ToolScheduling == ToolSchedulingSequential {
		for i, call := range calls {
			runOne(ctx, i, call)
			mu.Lock()
			stop := fatalErr != nil
			mu.Unlock()
			if stop {
				return nil, fatalErr
			}
		}
	} else {
		maxParallel := opts.MaxParallelTools
		if maxParallel <= 0 || maxParallel > len(calls) {
			maxParallel = len(calls)
		}
		batchCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		batchInvocation := ctx.WithContext(batchCtx)
		sem := make(chan struct{}, maxParallel)
		var wg sync.WaitGroup

		for i, call := range calls {
			sem <- struct{}{}
			mu.Lock()
			stopped := fatalErr != nil
			mu.Unlock()
			if stopped {
				<-sem
				break
			}

			wg.Add(1)
			go func(idx int, c *content.FunctionCall) {
				defer wg.Done()
				defer func() { <-sem }()
				if batchInvocation.Ended() {
					return
				}
				runOne(batchInvocation, idx, c)
				mu.Lock()
				terminal := fatalErr != nil && tool.IsTerminal(fatalErr)
				mu.Unlock()
				if terminal {
					cancel()
				}
			}(i, call)
		}
		wg.Wait()
	}

	if fatalErr != nil {
		return nil, fatalErr
	}

	var mergedActions session.EventActions
	for _, actions := range actionsList {
		if actions == nil {
			continue
		}
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

var ErrToolNotFound = errors.New("tool not found")

func (f *Flow) handleToolError(call *content.FunctionCall, err error) (map[string]any, bool) {
	if errors.Is(err, ErrToolNotFound) {
		var names []string
		for _, t := range f.allTools() {
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
		return nil, fmt.Errorf("tool %q: %w", call.Name, ErrToolNotFound)
	}

	timeout := f.ToolTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if maxDuration := f.ExecutionOptions.Limits.MaxToolDuration; maxDuration > 0 && maxDuration < timeout {
		timeout = maxDuration
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	timeoutCtx := ctx.WithContext(toolCtx)

	flowCtx := newToolContext(timeoutCtx, call.ID, actions)

	args := call.Args
	for _, cb := range f.BeforeToolCallbacks {
		r, err := cb(flowCtx, t, args)
		if err != nil {
			return nil, err
		}
		if r != nil {
			args = r
		}
	}
	for _, guard := range f.ExecutionOptions.BeforeToolGuards {
		if err := guard(flowCtx, t, args); err != nil {
			return nil, tool.NewDeniedError(err)
		}
	}

	processCtx := tool.ContextWithActions(flowCtx, actions)
	processCtx = tool.ContextWithState(processCtx, ctx.Session().State())
	result, err := t.Process(processCtx, args)

	if !tool.IsTerminal(err) {
		for _, cb := range f.AfterToolCallbacks {
			r, e := cb(flowCtx, t, args, result, err)
			if r != nil || e != nil {
				result = r
				err = e
				break
			}
		}
	}

	if err == nil {
		policy := f.ResultPolicy
		if policy == nil {
			policy = defaultResultPolicy
		}
		result, err = policy.Process(flowCtx, result)
		if err == nil {
			if maxBytes := f.ExecutionOptions.Limits.MaxToolResultBytes; maxBytes > 0 {
				size, sizeErr := toolResultBytes(result)
				if sizeErr != nil {
					err = fmt.Errorf("measure tool result: %w", sizeErr)
				} else if size > maxBytes {
					err = tool.NewDeniedError(fmt.Errorf("tool result exceeded %d bytes", maxBytes))
				}
			}
		}
	}

	if err != nil {
		if tool.IsTerminal(err) {
			return nil, err
		}
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
	for _, t := range f.allTools() {
		toolName := t.Name()
		if toolName == name || strings.ToLower(toolName) == lower {
			return t
		}
	}
	return nil
}

func (f *Flow) allTools() []tool.Tool {
	if len(f.Toolsets) == 0 {
		return f.Tools
	}

	tools := make([]tool.Tool, 0, len(f.Tools))
	seen := make(map[string]bool, len(f.Tools))
	for _, t := range f.Tools {
		if seen[t.Name()] {
			continue
		}
		seen[t.Name()] = true
		tools = append(tools, t)
	}
	for _, ts := range f.Toolsets {
		if ts == nil {
			continue
		}
		for _, t := range ts.Tools() {
			if seen[t.Name()] {
				continue
			}
			seen[t.Name()] = true
			tools = append(tools, t)
		}
	}
	return tools
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
	event.ModelVersion = resp.ModelVersion
	event.TurnComplete = resp.TurnComplete
	event.Interrupted = resp.Interrupted
	event.ErrorCode = resp.ErrorCode
	event.ErrorMessage = resp.ErrorMessage
	if resp.Content != nil {
		event.ModelResponse = resp.Content
	}
	return event
}

func (f *Flow) buildToolDeclarations() map[string]any {
	declarations := make(map[string]any)
	for _, t := range f.allTools() {
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
