package openai

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"sort"
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
		caps:   model.ModelCapabilities{ToolCall: true, StructuredOutput: true},
	}
}

func (a *Adapter) Name() string { return a.cfg.ProviderModelID }

func (a *Adapter) Capabilities() model.ModelCapabilities { return a.caps }

func (a *Adapter) GenerateContent(ctx context.Context, req *model.ModelRequest, stream bool) iter.Seq2[*model.ModelResponse, error] {
	if stream {
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
		return nil, fmt.Errorf("build openai request: %w", err)
	}

	maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.SingleAttempt, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := a.beforeAttempt(ctx, attempt); err != nil {
			return nil, err
		}
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/chat/completions", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			var redirectErr *internal.RedirectError
			if errors.As(err, &redirectErr) {
				return nil, fmt.Errorf("call openai api: %w", err)
			}
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call openai api: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var completion chatCompletionResponse
			maxResponseBytes := internal.StreamLimit(a.cfg.MaxResponseBytes, internal.DefaultMaxResponseBytes)
			if err := internal.DecodeJSONLimited(resp.Body, &completion, maxResponseBytes); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("decode openai response: %w", err)
			}
			resp.Body.Close()
			return fromOpenAIResponse(&completion), nil
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

	return nil, fmt.Errorf("openai api: max retries (%d) exceeded", maxRetries)
}

func (a *Adapter) generateStream(ctx context.Context, req *model.ModelRequest) iter.Seq2[*model.ModelResponse, error] {
	return func(yield func(*model.ModelResponse, error) bool) {
		body, err := a.buildRequestBody(req)
		if err != nil {
			yield(nil, fmt.Errorf("build openai request: %w", err))
			return
		}

		body.Stream = true
		body.StreamOptions = &streamOptions{IncludeUsage: true}

		maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.SingleAttempt, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)
		resp, err := a.openStream(ctx, body, maxRetries, initialWait, maxWait)
		if err != nil {
			yield(nil, err)
			return
		}
		defer resp.Body.Close()

		accumulator := newStreamAccumulator()
		scanner := newSSEScanner(resp.Body)

		maxStreamBytes := internal.StreamLimit(a.cfg.MaxStreamBytes, internal.DefaultMaxStreamBytes)
		maxToolCalls := internal.StreamLimit(a.cfg.MaxStreamToolCalls, internal.DefaultMaxStreamToolCalls)
		streamBytes := 0
		emitted := false
		sawFinishReason := false
		sawDone := false

		var usage *model.Usage
		var modelVersion string
		finishReason := model.FinishReasonStop

	scan:
		for scanner.Next() {
			event := scanner.Event()
			streamBytes += len(event.Data)
			if streamBytes > maxStreamBytes {
				yield(nil, newStreamError(model.StreamErrorLimit, emitted, fmt.Errorf("stream exceeded %d bytes", maxStreamBytes)))
				return
			}
			if strings.TrimSpace(event.Data) == "[DONE]" {
				sawDone = true
				break scan
			}
			if strings.TrimSpace(event.Data) == "" {
				continue
			}

			var chunk chatCompletionChunk
			if err := internal.DecodeJSONString(event.Data, &chunk); err != nil {
				yield(nil, newStreamError(model.StreamErrorMalformed, emitted, fmt.Errorf("decode stream chunk: %w", err)))
				return
			}

			if chunk.Usage != nil {
				usage = fromOpenAIUsage(chunk.Usage)
			}
			if chunk.Model != "" {
				modelVersion = chunk.Model
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			if choice.FinishReason != "" {
				sawFinishReason = true
				if fr := fromOpenAIFinishReason(choice.FinishReason); fr != model.FinishReasonUnknown {
					finishReason = fr
				}
			}
			parts := a.processChunk(choice, accumulator)
			if accumulator.toolCalls.count() > maxToolCalls {
				yield(nil, newStreamError(model.StreamErrorLimit, emitted, fmt.Errorf("stream exceeded %d tool calls", maxToolCalls)))
				return
			}

			if len(parts) > 0 {
				partialResp := &model.ModelResponse{
					Content: content.NewContent(content.RoleModel, parts...),
					Partial: true,
				}
				if !yield(partialResp, nil) {
					return
				}
				emitted = true
			}
		}

		if err := scanner.Err(); err != nil {
			yield(nil, newStreamError(model.StreamErrorIncomplete, emitted, fmt.Errorf("scan sse stream: %w", err)))
			return
		}
		if !sawDone && !sawFinishReason {
			yield(nil, newStreamError(model.StreamErrorIncomplete, emitted, fmt.Errorf("stream ended without finish reason or done marker")))
			return
		}

		finalParts, err := accumulator.finalize()
		if err != nil {
			yield(nil, newStreamError(model.StreamErrorInvalidToolCall, emitted, err))
			return
		}

		finalResp := &model.ModelResponse{
			TurnComplete: true,
			Partial:      false,
			Usage:        usage,
			FinishReason: finishReason,
			ModelVersion: modelVersion,
		}
		if len(finalParts) > 0 {
			finalResp.Content = content.NewContent(content.RoleModel, finalParts...)
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
		Provider:   "openai",
		StatusCode: statusCode,
		Code:       code,
		Retryable:  internal.IsRetryableStatus(statusCode),
		RetryAfter: internal.RetryAfter(resp),
		RequestID:  internal.ProviderRequestID(resp.Header),
	}
}

func newStreamError(kind model.StreamErrorKind, interrupted bool, err error) error {
	return fmt.Errorf("openai stream: %w", &model.StreamError{Kind: kind, Interrupted: interrupted, Err: err})
}

func (a *Adapter) openStream(ctx context.Context, body *chatCompletionRequest, maxRetries int, initialWait, maxWait time.Duration) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := a.beforeAttempt(ctx, attempt); err != nil {
			return nil, err
		}
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/chat/completions", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			var redirectErr *internal.RedirectError
			if errors.As(err, &redirectErr) {
				return nil, fmt.Errorf("call openai streaming api: %w", err)
			}
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call openai streaming api: %w", err)
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

	return nil, fmt.Errorf("openai streaming api: max retries (%d) exceeded", maxRetries)
}

func (a *Adapter) processChunk(choice chatCompletionChoice, acc *streamAccumulator) []content.Part {
	var parts []content.Part

	if choice.Delta != nil {
		if choice.Delta.Content != "" {
			acc.text.WriteString(choice.Delta.Content)
			parts = append(parts, content.NewTextPart(choice.Delta.Content))
		}
		if choice.Delta.ReasoningContent != "" {
			acc.thinking.WriteString(choice.Delta.ReasoningContent)
			parts = append(parts, content.NewThinkingPart(choice.Delta.ReasoningContent))
		}
		for _, tc := range choice.Delta.ToolCalls {
			acc.toolCalls.feed(tc.Index, tc)
		}
	}

	return parts
}

var _ model.Model = (*Adapter)(nil)

type streamAccumulator struct {
	toolCalls *toolCallAccumulator
	text      strings.Builder
	thinking  strings.Builder
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{toolCalls: newToolCallAccumulator()}
}

func (a *streamAccumulator) finalize() ([]content.Part, error) {
	parts := make([]content.Part, 0, 3)
	if a.thinking.Len() > 0 {
		parts = append(parts, content.NewThinkingPart(a.thinking.String()))
	}
	if a.text.Len() > 0 {
		parts = append(parts, content.NewTextPart(a.text.String()))
	}
	toolParts, err := a.toolCalls.finalize()
	if err != nil {
		return nil, err
	}
	return append(parts, toolParts...), nil
}

type toolCallAccumulator struct {
	mu          sync.Mutex
	calls       map[int]*content.FunctionCall
	argsBuffers map[int]*strings.Builder
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{
		calls:       make(map[int]*content.FunctionCall),
		argsBuffers: make(map[int]*strings.Builder),
	}
}

func (a *toolCallAccumulator) feed(index int, chunk toolCallChunk) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.calls[index]; !ok {
		a.calls[index] = &content.FunctionCall{
			ID:   chunk.ID,
			Name: chunk.Function.Name,
			Args: make(map[string]any),
		}
		a.argsBuffers[index] = &strings.Builder{}
	}

	if chunk.ID != "" {
		a.calls[index].ID = chunk.ID
	}
	if chunk.Function.Name != "" {
		a.calls[index].Name = chunk.Function.Name
	}
	a.argsBuffers[index].WriteString(chunk.Function.Arguments)
}

func (a *toolCallAccumulator) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

func (a *toolCallAccumulator) finalize() ([]content.Part, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	indices := make([]int, 0, len(a.calls))
	for i := range a.calls {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	parts := make([]content.Part, 0, len(indices))
	for _, i := range indices {
		fc := a.calls[i]
		buf, ok := a.argsBuffers[i]
		if !ok {
			return nil, fmt.Errorf("tool call %d has no argument buffer", i)
		}
		if fc.ID == "" {
			return nil, fmt.Errorf("tool call %d is missing an id", i)
		}
		if fc.Name == "" {
			return nil, fmt.Errorf("tool call %d is missing a name", i)
		}
		args, err := parseArgsJSONStrict(buf.String())
		if err != nil {
			return nil, fmt.Errorf("tool call %q has invalid arguments: %w", fc.Name, err)
		}
		fc.Args = args
		parts = append(parts, content.Part{FunctionCall: fc})
	}
	return parts, nil
}
