package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	BaseURL          string
	APIKey           string
	ProviderModelID  string
	MaxRetries       int
	InitialRetryWait time.Duration
	MaxRetryWait     time.Duration
}

type Adapter struct {
	cfg    Config
	client *http.Client
	caps   model.ModelCapabilities
}

func New(cfg Config) *Adapter {
	return &Adapter{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
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

	maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/messages", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call anthropic api: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var msgResp messagesResponse
			if err := internal.DecodeJSON(resp.Body, &msgResp); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("decode anthropic response: %w", err)
			}
			resp.Body.Close()
			return fromAnthropicResponse(&msgResp), nil
		}

		statusCode := resp.StatusCode
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if !internal.IsRetryableStatus(statusCode) {
			return nil, fmt.Errorf("anthropic api returned HTTP %d: %s", statusCode, errBody)
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

		maxRetries, initialWait, maxWait := internal.RetryConfig(a.cfg.MaxRetries, a.cfg.InitialRetryWait, a.cfg.MaxRetryWait)
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
		var finishReason model.FinishReason

		for scanner.Next() {
			event := scanner.Event()

			var sseEvent anthropicSSE
			if err := internal.DecodeJSONString(event.Data, &sseEvent); err != nil {
				continue
			}

			switch sseEvent.Type {
			case "content_block_start":
				if sseEvent.ContentBlock != nil && sseEvent.ContentBlock.Type == "tool_use" {
					acc.start(sseEvent.Index, sseEvent.ContentBlock.ID, sseEvent.ContentBlock.Name)
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
					case "thinking_delta":
						thinkingText.WriteString(sseEvent.Delta.Thinking)
						partialResp := &model.ModelResponse{
							Content: content.NewContent(content.RoleModel, content.NewThinkingPart(sseEvent.Delta.Thinking)),
							Partial: true,
						}
						if !yield(partialResp, nil) {
							return
						}
					case "input_json_delta":
						acc.feedDelta(sseEvent.Index, sseEvent.Delta.PartialJSON)
					}
				}

			case "content_block_stop":
				acc.finalize(sseEvent.Index)

			case "message_delta":
				if sseEvent.Usage != nil {
					usage = &model.Usage{
						CompletionTokens: sseEvent.Usage.OutputTokens,
					}
				}
				if sseEvent.Delta != nil && sseEvent.Delta.StopReason != "" {
					finishReason = fromAnthropicStopReason(sseEvent.Delta.StopReason)
				}

			case "message_start":
				if sseEvent.Message != nil && sseEvent.Message.Usage != nil {
					usage = &model.Usage{
						PromptTokens:     sseEvent.Message.Usage.InputTokens,
						CompletionTokens: sseEvent.Message.Usage.OutputTokens,
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("scan sse stream: %w", err))
			return
		}

		toolCallParts := acc.parts()

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

func (a *Adapter) openStream(ctx context.Context, body *messagesRequest, maxRetries int, initialWait, maxWait time.Duration) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := internal.WaitBeforeRetry(ctx, attempt, initialWait, maxWait); err != nil {
			return nil, err
		}

		httpReq, err := a.newHTTPRequest(ctx, "POST", "/messages", body)
		if err != nil {
			return nil, fmt.Errorf("create http request: %w", err)
		}

		resp, err := a.client.Do(httpReq)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("call anthropic streaming api: %w", err)
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
			return nil, fmt.Errorf("anthropic streaming api returned HTTP %d: %s", statusCode, errBody)
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

func (a *anthropicToolCallAccumulator) finalize(index int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fc, ok := a.calls[index]
	if !ok {
		return
	}
	args := make(map[string]any)
	if builder := a.argsBufs[index]; builder != nil && builder.Len() > 0 {
		buf := builder.String()
		if err := json.Unmarshal([]byte(buf), &args); err != nil {
			args = map[string]any{"raw": buf}
		}
	}
	fc.Args = args
	a.finals[index] = content.Part{FunctionCall: fc}
}

func (a *anthropicToolCallAccumulator) parts() []content.Part {
	a.mu.Lock()
	defer a.mu.Unlock()
	var result []content.Part
	for i := 0; i < len(a.finals); i++ {
		if p, ok := a.finals[i]; ok {
			result = append(result, p)
		}
	}
	return result
}
