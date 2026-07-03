package agent

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"

	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/session"
)

type Metadata interface {
	Name() string
	Description() string
}

type Runner interface {
	Run(ctx InvocationContext) iter.Seq2[*session.Event, error]
}

type Tree interface {
	SubAgents() []Agent
	FindAgent(name string) Agent
	FindSubAgent(name string) Agent
}

type Agent interface {
	Metadata
	Runner
	Tree
}

type BeforeAgentCallback func(CallbackContext) (*content.Content, error)

type AfterAgentCallback func(CallbackContext) (*content.Content, error)

type AgentOption func(*baseAgent)

type baseAgent struct {
	name                 string
	description          string
	subAgents            []Agent
	beforeAgentCallbacks []BeforeAgentCallback
	afterAgentCallbacks  []AfterAgentCallback
	run                  func(InvocationContext) iter.Seq2[*session.Event, error]
}

func New(opts ...AgentOption) Agent {
	a := &baseAgent{subAgents: []Agent{}}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func WithName(name string) AgentOption {
	return func(a *baseAgent) {
		a.name = name
	}
}

func WithDescription(desc string) AgentOption {
	return func(a *baseAgent) {
		a.description = desc
	}
}

func WithSubAgents(subAgents ...Agent) AgentOption {
	return func(a *baseAgent) {
		a.subAgents = append(a.subAgents, subAgents...)
	}
}

func WithRun(run func(InvocationContext) iter.Seq2[*session.Event, error]) AgentOption {
	return func(a *baseAgent) {
		a.run = run
	}
}

func WithBeforeAgentCallbacks(cb ...BeforeAgentCallback) AgentOption {
	return func(a *baseAgent) {
		a.beforeAgentCallbacks = append(a.beforeAgentCallbacks, cb...)
	}
}

func WithAfterAgentCallbacks(cb ...AfterAgentCallback) AgentOption {
	return func(a *baseAgent) {
		a.afterAgentCallbacks = append(a.afterAgentCallbacks, cb...)
	}
}

func (a *baseAgent) Name() string {
	return a.name
}

func (a *baseAgent) Description() string {
	return a.description
}

func (a *baseAgent) SubAgents() []Agent {
	return a.subAgents
}

func (a *baseAgent) FindAgent(name string) Agent {
	if a.name == name {
		return a
	}
	return a.FindSubAgent(name)
}

func (a *baseAgent) FindSubAgent(name string) Agent {
	for _, sub := range a.subAgents {
		if result := sub.FindAgent(name); result != nil {
			return result
		}
	}
	return nil
}

func (a *baseAgent) Run(ctx InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		invCtx := &invocationContext{
			Context:       ctx,
			agent:         a,
			artifactSvc:   ctx.ArtifactService(),
			memorySvc:     ctx.MemoryService(),
			session:       ctx.Session(),
			invocationID:  ctx.InvocationID(),
			branch:        ctx.Branch(),
			userContent:   ctx.UserContent(),
			runConfig:     ctx.RunConfig(),
			endInvocation: &atomic.Bool{},
		}
		if ctx.Ended() {
			invCtx.EndInvocation()
		}

		if event, err := runBeforeAgentCallbacks(invCtx); event != nil || err != nil {
			if !yield(event, err) {
				return
			}
		}

		if invCtx.Ended() {
			return
		}

		if a.run != nil {
			for event, err := range a.run(invCtx) {
				if !yield(event, err) {
					return
				}
			}
		}

		if invCtx.Ended() {
			return
		}

		if event, err := runAfterAgentCallbacks(invCtx); event != nil || err != nil {
			yield(event, err)
		}
	}
}

type callbackContext struct {
	context.Context
	invocationContext InvocationContext
	actions           *session.EventActions
}

func (c *callbackContext) AgentName() string {
	return c.invocationContext.Agent().Name()
}

func (c *callbackContext) State() session.State {
	return &callbackContextState{ctx: c}
}

func (c *callbackContext) ReadonlyState() session.ReadonlyState {
	return &callbackContextState{ctx: c}
}

func (c *callbackContext) Artifacts() artifact.Service {
	return c.invocationContext.ArtifactService()
}

func (c *callbackContext) InvocationID() string {
	return c.invocationContext.InvocationID()
}

func (c *callbackContext) UserContent() *content.Content {
	return c.invocationContext.UserContent()
}

func (c *callbackContext) AppName() string {
	return c.invocationContext.Session().AppName()
}

func (c *callbackContext) Branch() string {
	return c.invocationContext.Branch()
}

func (c *callbackContext) SessionID() string {
	return c.invocationContext.Session().ID()
}

func (c *callbackContext) UserID() string {
	return c.invocationContext.Session().UserID()
}

var _ CallbackContext = (*callbackContext)(nil)

var _ ReadonlyContext = (*callbackContext)(nil)

type callbackContextState struct {
	ctx *callbackContext
}

func (s *callbackContextState) Get(key string) (any, error) {
	if s.ctx.actions != nil && s.ctx.actions.StateDelta != nil {
		if val, ok := s.ctx.actions.StateDelta[key]; ok {
			return val, nil
		}
	}
	return s.ctx.invocationContext.Session().State().Get(key)
}

func (s *callbackContextState) Set(key string, val any) error {
	if s.ctx.actions != nil && s.ctx.actions.StateDelta != nil {
		s.ctx.actions.StateDelta[key] = val
	}
	return s.ctx.invocationContext.Session().State().Set(key, val)
}

func (s *callbackContextState) All() iter.Seq2[string, any] {
	return s.ctx.invocationContext.Session().State().All()
}

var _ session.State = (*callbackContextState)(nil)

var _ session.ReadonlyState = (*callbackContextState)(nil)

func NewInvocationContext(
	ctx context.Context,
	ag Agent,
	artifactSvc artifact.Service,
	memorySvc memory.Service,
	sess session.Session,
	invocationID string,
	branch string,
	userContent *content.Content,
	runConfig *RunConfig,
) InvocationContext {
	return &invocationContext{
		Context:       ctx,
		agent:         ag,
		artifactSvc:   artifactSvc,
		memorySvc:     memorySvc,
		session:       sess,
		invocationID:  invocationID,
		branch:        branch,
		userContent:   userContent,
		runConfig:     runConfig,
		endInvocation: &atomic.Bool{},
	}
}

type invocationContext struct {
	context.Context
	agent         Agent
	artifactSvc   artifact.Service
	memorySvc     memory.Service
	session       session.Session
	invocationID  string
	branch        string
	userContent   *content.Content
	runConfig     *RunConfig
	endInvocation *atomic.Bool
}

func (c *invocationContext) Agent() Agent {
	return c.agent
}

func (c *invocationContext) ArtifactService() artifact.Service {
	return c.artifactSvc
}

func (c *invocationContext) MemoryService() memory.Service {
	return c.memorySvc
}

func (c *invocationContext) Session() session.Session {
	return c.session
}

func (c *invocationContext) InvocationID() string {
	return c.invocationID
}

func (c *invocationContext) Branch() string {
	return c.branch
}

func (c *invocationContext) UserContent() *content.Content {
	return c.userContent
}

func (c *invocationContext) RunConfig() *RunConfig {
	return c.runConfig
}

func (c *invocationContext) EndInvocation() {
	c.endInvocation.Store(true)
}

func (c *invocationContext) Ended() bool {
	return c.endInvocation.Load()
}

func (c *invocationContext) WithContext(ctx context.Context) InvocationContext {
	newCtx := *c
	newCtx.Context = ctx
	return &newCtx
}

var _ InvocationContext = (*invocationContext)(nil)

func runBeforeAgentCallbacks(ctx InvocationContext) (*session.Event, error) {
	agent := ctx.Agent()
	cbCtx := &callbackContext{
		Context:           ctx,
		invocationContext: ctx,
		actions:           &session.EventActions{StateDelta: make(map[string]any)},
	}

	for _, callback := range agent.(*baseAgent).beforeAgentCallbacks {
		content, err := callback(cbCtx)
		if err != nil {
			return nil, fmt.Errorf("before agent callback failed: %w", err)
		}
		if content == nil {
			continue
		}
		event := session.NewEvent(ctx.InvocationID())
		event.ModelResponse = content
		event.Author = agent.Name()
		event.Branch = ctx.Branch()
		event.Actions = *cbCtx.actions
		ctx.EndInvocation()
		return event, nil
	}

	if len(cbCtx.actions.StateDelta) > 0 {
		event := session.NewEvent(ctx.InvocationID())
		event.Author = agent.Name()
		event.Branch = ctx.Branch()
		event.Actions = *cbCtx.actions
		return event, nil
	}
	return nil, nil
}

func runAfterAgentCallbacks(ctx InvocationContext) (*session.Event, error) {
	agent := ctx.Agent()
	cbCtx := &callbackContext{
		Context:           ctx,
		invocationContext: ctx,
		actions:           &session.EventActions{StateDelta: make(map[string]any)},
	}

	for _, callback := range agent.(*baseAgent).afterAgentCallbacks {
		content, err := callback(cbCtx)
		if err != nil {
			return nil, fmt.Errorf("after agent callback failed: %w", err)
		}
		if content == nil {
			continue
		}
		event := session.NewEvent(ctx.InvocationID())
		event.ModelResponse = content
		event.Author = agent.Name()
		event.Branch = ctx.Branch()
		event.Actions = *cbCtx.actions
		return event, nil
	}

	if len(cbCtx.actions.StateDelta) > 0 {
		event := session.NewEvent(ctx.InvocationID())
		event.Author = agent.Name()
		event.Branch = ctx.Branch()
		event.Actions = *cbCtx.actions
		return event, nil
	}
	return nil, nil
}
