package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	internal "github.com/codeownersnet/goa/provider/internal"
)

type Config struct {
	BaseURL            string
	APIKey             string
	ProviderModelID    string
	MaxRetries         int
	SingleAttempt      bool
	MaxStreamBytes     int
	MaxStreamToolCalls int
	MaxResponseBytes   int
	MaxErrorBodyBytes  int
	StreamIdleTimeout  time.Duration
	DisableRedirects   bool
	CheckRedirect      func(*http.Request, []*http.Request) error
	BeforeAttempt      func(context.Context, int) error
	InitialRetryWait   time.Duration
	MaxRetryWait       time.Duration
	HTTPClient         *http.Client
}

type Adapter struct {
	cfg    Config
	client *http.Client
	caps   model.ModelCapabilities
}

func New(cfg Config) *Adapter {
	client := internal.NewHTTPClient(cfg.HTTPClient, cfg.DisableRedirects, cfg.CheckRedirect)
	return &Adapter{
		cfg:    cfg,
		client: client,
		caps:   model.ModelCapabilities{ToolCall: true, StructuredOutput: false, Reasoning: true},
	}
}

func (a *Adapter) Name() string { return a.cfg.ProviderModelID }

func (a *Adapter) Capabilities() model.ModelCapabilities { return a.caps }

func (a *Adapter) GenerateContent(ctx context.Context, req *model.ModelRequest, stream bool) iter.Seq2[*model.ModelResponse, error] {
	cfg := req.Config
	if cfg == nil {
		cfg = &model.GenerateConfig{}
	}

	forcedStream := stream
	if cfg.ThinkingConfig != nil && cfg.ThinkingConfig.BudgetTokens > 0 {
		forcedStream = true
	}

	if forcedStream {
		return a.generateStream(ctx, req)
	}
	return func(yield func(*model.ModelResponse, error) bool) {
		resp, err := a.generate(ctx, req)
		yield(resp, err)
	}
}

func (a *Adapter) generate(ctx context.Context, req *model.ModelRequest) (*model.ModelResponse, error) {
	body, err := a.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("build anthropic request: %w", err)
	}

	maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.SingleAttempt, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := a.beforeAttempt(ctx, attempt); err != nil {
			return nil, err
		}
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/messages", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			var redirectErr *internal.RedirectError
			if errors.As(err, &redirectErr) {
				return nil, fmt.Errorf("call anthropic api: %w", err)
			}
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call anthropic api: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var msgResp messagesResponse
			maxResponseBytes := internal.StreamLimit(a.cfg.MaxResponseBytes, internal.DefaultMaxResponseBytes)
			if err := internal.DecodeJSONLimited(resp.Body, &msgResp, maxResponseBytes); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("decode anthropic response: %w", err)
			}
			resp.Body.Close()
			return fromAnthropicResponse(&msgResp), nil
		}

		statusCode := resp.StatusCode
		info := internal.ReadErrorInfo(resp.Body, internal.StreamLimit(a.cfg.MaxErrorBodyBytes, internal.DefaultMaxErrorBodyBytes))
		resp.Body.Close()

		if !internal.IsRetryableStatus(statusCode) {
			return nil, a.providerError(resp, statusCode, info.Code)
		}

		if after := internal.RetryAfter(resp); after > 0 {
			initialWait = after
		}
	}

	return nil, fmt.Errorf("anthropic api: max retries (%d) exceeded", maxRetries)
}

func (a *Adapter) generateStream(ctx context.Context, req *model.ModelRequest) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		body, err := a.buildRequestBody(req)
		if err != nil {
			yield(nil, fmt.Errorf("build anthropic request: %w", err))
			return
		}

		body.Stream = true

		maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.SingleAttempt, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)
		resp, err := a.openStream(ctx, body, maxRetries, initialWait, maxWait)
		if err != nil {
			yield(nil, err)
			return
		}
		defer resp.Body.Close()

		scanner := newSSEScanner(resp.Body)
		var thinkingText strings.Builder
		var textContent strings.Builder
		acc := newAnthropicToolCallAccumulator()
		var usage *model.Usage
		finishReason := model.FinishReasonStop
		maxStreamBytes := internal.StreamLimit(a.cfg.MaxStreamBytes, internal.DefaultMaxStreamBytes)
		maxToolCalls := internal.StreamLimit(a.cfg.MaxStreamToolCalls, internal.DefaultMaxStreamToolCalls)
		streamBytes := 0
		emitted := false
		sawMessageStop := false
		sawStopReason := false

		for scanner.Next() {
			event := scanner.Event()
			streamBytes += len(event.Data)
			if streamBytes > maxStreamBytes {
				yield(nil, newAnthropicStreamError(model.StreamErrorLimit, emitted, fmt.Errorf("stream exceeded %d bytes", maxStreamBytes)))
				return
			}

			if strings.TrimSpace(event.Data) == "" {
				continue
			}

			var sseEvent anthropicSSE
			if err := internal.DecodeJSONString(event.Data, &sseEvent); err != nil {
				yield(nil, newAnthropicStreamError(model.StreamErrorMalformed, emitted, fmt.Errorf("decode stream event: %w", err)))
				return
			}

			switch sseEvent.Type {
			case "content_block_start":
				if sseEvent.ContentBlock != nil && sseEvent.ContentBlock.Type == "tool_use" {
					acc.start(sseEvent.Index, sseEvent.ContentBlock.ID, sseEvent.ContentBlock.Name)
					if acc.count() > maxToolCalls {
						yield(nil, newAnthropicStreamError(model.StreamErrorLimit, emitted, fmt.Errorf("stream exceeded %d tool calls", maxToolCalls)))
						return
					}
				}

			case "content_block_delta":
				if sseEvent.Delta != nil {
					switch sseEvent.Delta.Type {
					case "text_delta":
						textContent.WriteString(sseEvent.Delta.Text)
						partialResp := &model.ModelResponse{
							Content: content.NewContent(content.RoleModel, content.NewTextPart(sseEvent.Delta.Text)),
							Partial: true,
						}
						if !yield(partialResp, nil) {
							return
						}
						emitted = true
					case "thinking_delta":
						thinkingText.WriteString(sseEvent.Delta.Thinking)
						partialResp := &model.ModelResponse{
							Content: content.NewContent(content.RoleModel, content.NewThinkingPart(sseEvent.Delta.Thinking)),
							Partial: true,
						}
						if !yield(partialResp, nil) {
							return
						}
						emitted = true
					case "input_json_delta":
						acc.feedDelta(sseEvent.Index, sseEvent.Delta.PartialJSON)
					}
				}

			case "content_block_stop":
				acc.finalize(sseEvent.Index)

			case "message_delta":
				if sseEvent.Usage != nil {
					if usage == nil {
						usage = &model.Usage{}
					}
					usage.CompletionTokens = sseEvent.Usage.OutputTokens
				}
				if sseEvent.Delta != nil && sseEvent.Delta.StopReason != "" {
					finishReason = fromAnthropicStopReason(sseEvent.Delta.StopReason)
					sawStopReason = true
				}

			case "message_stop":
				sawMessageStop = true

			case "message_start":
				if sseEvent.Message != nil && sseEvent.Message.Usage != nil {
					usage = fromAnthropicUsage(sseEvent.Message.Usage)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			yield(nil, newAnthropicStreamError(model.StreamErrorIncomplete, emitted, fmt.Errorf("scan sse stream: %w", err)))
			return
		}
		if !sawMessageStop && !sawStopReason {
			yield(nil, newAnthropicStreamError(model.StreamErrorIncomplete, emitted, fmt.Errorf("stream ended without message_stop or stop reason")))
			return
		}

		toolCallParts, err := acc.parts()
		if err != nil {
			yield(nil, newAnthropicStreamError(model.StreamErrorInvalidToolCall, emitted, err))
			return
		}

		var allParts []content.Part
		if thinkingText.Len() > 0 {
			allParts = append(allParts, content.NewThinkingPart(thinkingText.String()))
		}
		if textContent.Len() > 0 {
			allParts = append(allParts, content.NewTextPart(textContent.String()))
		}
		allParts = append(allParts, toolCallParts...)

		finalResp := &model.ModelResponse{
			Content:      content.NewContent(content.RoleModel, allParts...),
			Usage:        usage,
			FinishReason: finishReason,
			TurnComplete: true,
			Partial:      false,
		}
		yield(finalResp, nil)
	}
}

func (a *Adapter) beforeAttempt(ctx context.Context, attempt int) error {
	if a.cfg.BeforeAttempt != nil {
		return a.cfg.BeforeAttempt(ctx, attempt)
	}
	return nil
}

func (a *Adapter) providerError(resp *http.Response, statusCode int, code string) error {
	return &model.ProviderError{
		Provider:   "anthropic",
		StatusCode: statusCode,
		Code:       code,
		Retryable:  internal.IsRetryableStatus(statusCode),
		RetryAfter: internal.RetryAfter(resp),
		RequestID:  internal.ProviderRequestID(resp.Header),
	}
}

func newAnthropicStreamError(kind model.StreamErrorKind, interrupted bool, err error) error {
	return fmt.Errorf("anthropic stream: %w", &model.StreamError{Kind: kind, Interrupted: interrupted, Err: err})
}

func (a *Adapter) openStream(ctx context.Context, body *messagesRequest, maxRetries int, initialWait, maxWait time.Duration) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := a.beforeAttempt(ctx, attempt); err != nil {
			return nil, err
		}
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/messages", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			var redirectErr *internal.RedirectError
			if errors.As(err, &redirectErr) {
				return nil, fmt.Errorf("call anthropic streaming api: %w", err)
			}
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call anthropic streaming api: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			if a.cfg.StreamIdleTimeout > 0 {
				resp.Body = internal.NewIdleTimeoutReader(resp.Body, a.cfg.StreamIdleTimeout)
			}
			return resp, nil
		}

		statusCode := resp.StatusCode
		if after := internal.RetryAfter(resp); after > 0 {
			initialWait = after
		}
		info := internal.ReadErrorInfo(resp.Body, internal.StreamLimit(a.cfg.MaxErrorBodyBytes, internal.DefaultMaxErrorBodyBytes))
		resp.Body.Close()

		if !internal.IsRetryableStatus(statusCode) {
			return nil, a.providerError(resp, statusCode, info.Code)
		}
	}

	return nil, fmt.Errorf("anthropic streaming api: max retries (%d) exceeded", maxRetries)
}

var _ model.Model = (*Adapter)(nil)

type anthropicToolCallAccumulator struct {
	mu       sync.Mutex
	calls    map[int]*content.FunctionCall
	argsBufs map[int]*strings.Builder
	finals   map[int]content.Part
	err      error
}

func newAnthropicToolCallAccumulator() *anthropicToolCallAccumulator {
	return &anthropicToolCallAccumulator{
		calls:    make(map[int]*content.FunctionCall),
		argsBufs: make(map[int]*strings.Builder),
		finals:   make(map[int]content.Part),
	}
}

func (a *anthropicToolCallAccumulator) start(index int, id, name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls[index] = &content.FunctionCall{ID: id, Name: name, Args: make(map[string]any)}
	a.argsBufs[index] = &strings.Builder{}
}

func (a *anthropicToolCallAccumulator) feedDelta(index int, partialJSON string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.argsBufs[index]; !ok {
		a.argsBufs[index] = &strings.Builder{}
	}
	a.argsBufs[index].WriteString(partialJSON)
}

func (a *anthropicToolCallAccumulator) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

func (a *anthropicToolCallAccumulator) finalize(index int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.finals[index]; ok {
		return
	}
	fc, ok := a.calls[index]
	if !ok {
		return
	}
	if fc.ID == "" {
		a.err = fmt.Errorf("tool call %d is missing an id", index)
		return
	}
	if fc.Name == "" {
		a.err = fmt.Errorf("tool call %d is missing a name", index)
		return
	}
	args, err := parseAnthropicArgs(a.argsBufs[index].String())
	if err != nil {
		a.err = fmt.Errorf("tool call %q has invalid arguments: %w", fc.Name, err)
		return
	}
	fc.Args = args
	a.finals[index] = content.Part{FunctionCall: fc}
}

func (a *anthropicToolCallAccumulator) parts() ([]content.Part, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return nil, a.err
	}
	for index := range a.calls {
		if _, ok := a.finals[index]; !ok {
			return nil, fmt.Errorf("tool call %d was not completed", index)
		}
	}
	var result []content.Part
	for i := 0; i < len(a.finals); i++ {
		if p, ok := a.finals[i]; ok {
			result = append(result, p)
		}
	}
	return result, nil
}

func parseAnthropicArgs(s string) (map[string]any, error) {
	if strings.TrimSpace(s) == "" {
		return make(map[string]any), nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(s), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = make(map[string]any)
	}
	return args, nil
}
