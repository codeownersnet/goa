package openai

import (
	"context"
	"fmt"
	"io"
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
	BaseURL          string
	APIKey           string
	ProviderModelID  string
	MaxRetries       int
	InitialRetryWait time.Duration
	MaxRetryWait     time.Duration
	HTTPClient       *http.Client
}

type Adapter struct {
	cfg    Config
	client *http.Client
	caps   model.ModelCapabilities
}

func New(cfg Config) *Adapter {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
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

	maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/chat/completions", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call openai api: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var completion chatCompletionResponse
			if err := internal.DecodeJSON(resp.Body, &completion); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("decode openai response: %w", err)
			}
			resp.Body.Close()
			return fromOpenAIResponse(&completion), nil
		}

		statusCode := resp.StatusCode
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if !internal.IsRetryableStatus(statusCode) {
			return nil, fmt.Errorf("openai api returned HTTP %d: %s", statusCode, errBody)
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

		maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)
		resp, err := a.openStream(ctx, body, maxRetries, initialWait, maxWait)
		if err != nil {
			yield(nil, err)
			return
		}
		defer resp.Body.Close()

		accumulator := newToolCallAccumulator()
		scanner := newSSEScanner(resp.Body)

		var usage *model.Usage
		finishReason := model.FinishReasonStop

	scan:
		for scanner.Next() {
			event := scanner.Event()
			if event.Data == "[DONE]" {
				break scan
			}

			var chunk chatCompletionChunk
			if err := internal.DecodeJSONString(event.Data, &chunk); err != nil {
				continue
			}

			if chunk.Usage != nil {
				usage = fromOpenAIUsage(chunk.Usage)
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			if fr := fromOpenAIFinishReason(choice.FinishReason); fr != model.FinishReasonUnknown {
				finishReason = fr
			}
			parts := a.processChunk(choice, accumulator)

			if len(parts) > 0 {
				partialResp := &model.ModelResponse{
					Content: content.NewContent(content.RoleModel, parts...),
					Partial: true,
				}
				if !yield(partialResp, nil) {
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("scan sse stream: %w", err))
			return
		}

		finalResp := &model.ModelResponse{
			TurnComplete: true,
			Partial:      false,
			Usage:        usage,
			FinishReason: finishReason,
		}
		if final := accumulator.finalize(); len(final) > 0 {
			finalResp.Content = content.NewContent(content.RoleModel, final...)
		}
		yield(finalResp, nil)
	}
}

func (a *Adapter) openStream(ctx context.Context, body *chatCompletionRequest, maxRetries int, initialWait, maxWait time.Duration) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/chat/completions", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call openai streaming api: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		statusCode := resp.StatusCode
		if after := internal.RetryAfter(resp); after > 0 {
			initialWait = after
		}
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if !internal.IsRetryableStatus(statusCode) {
			return nil, fmt.Errorf("openai streaming api returned HTTP %d: %s", statusCode, errBody)
		}
	}

	return nil, fmt.Errorf("openai streaming api: max retries (%d) exceeded", maxRetries)
}

func (a *Adapter) processChunk(choice chatCompletionChoice, acc *toolCallAccumulator) []content.Part {
	var parts []content.Part

	if choice.Delta != nil {
		if choice.Delta.Content != "" {
			parts = append(parts, content.NewTextPart(choice.Delta.Content))
		}
		if choice.Delta.ReasoningContent != "" {
			parts = append(parts, content.NewThinkingPart(choice.Delta.ReasoningContent))
		}
		for _, tc := range choice.Delta.ToolCalls {
			acc.feed(tc.Index, tc)
		}
	}

	return parts
}

var _ model.Model = (*Adapter)(nil)

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

func (a *toolCallAccumulator) finalize() []content.Part {
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
			continue
		}
		fc.Args = parseArgsJSON(buf.String())
		parts = append(parts, content.Part{FunctionCall: fc})
	}
	return parts
}
