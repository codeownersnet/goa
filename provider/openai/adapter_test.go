package openai

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func collectStream(seq iter.Seq2[*model.ModelResponse, error]) ([]*model.ModelResponse, error) {
	var resps []*model.ModelResponse
	for resp, err := range seq {
		if err != nil {
			return resps, err
		}
		resps = append(resps, resp)
	}
	return resps, nil
}

func userRequest() *model.ModelRequest {
	return &model.ModelRequest{
		Contents: []*content.Content{content.NewTextContent("hi", content.RoleUser)},
	}
}

func TestGenerateNonStreamingSuccess(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer k", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"id": "chatcmpl-1",
			"model": "m",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15, "prompt_tokens_details": {"cached_tokens": 4}}
		}`)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.NoError(t, err)
	require.Len(t, resps, 1)

	resp := resps[0]
	require.NotNil(t, resp.Content)
	assert.Equal(t, content.RoleModel, resp.Content.Role)
	require.Len(t, resp.Content.Parts, 1)
	require.NotNil(t, resp.Content.Parts[0].Text)
	assert.Equal(t, "Hello!", resp.Content.Parts[0].Text.Text)

	require.NotNil(t, resp.Usage)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
	assert.Equal(t, 4, resp.Usage.CacheReadTokens)

	assert.Equal(t, model.FinishReasonStop, resp.FinishReason)
	assert.Equal(t, "m", resp.ModelVersion)
	assert.False(t, resp.Partial)
	assert.False(t, resp.TurnComplete)
	assert.Equal(t, int32(1), hits.Load())
}

func TestGenerateNonRetryableError(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error": {"message": "invalid request"}}`)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.Error(t, err)
	var providerErr *model.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusBadRequest, providerErr.StatusCode)
	assert.NotContains(t, err.Error(), "invalid request")
	assert.Equal(t, int32(1), hits.Load())
}

func TestGenerateSingleAttemptDoesNotRetry(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.WriteHeader(http.StatusTooManyRequests)
				io.WriteString(w, `{"error": {"message": "slow down"}}`)
			}))
			defer server.Close()

			a := New(Config{
				BaseURL:          server.URL,
				APIKey:           "k",
				ProviderModelID:  "m",
				MaxRetries:       5,
				SingleAttempt:    true,
				InitialRetryWait: time.Millisecond,
				MaxRetryWait:     5 * time.Millisecond,
			})
			_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), stream))
			require.Error(t, err)
			assert.Equal(t, int32(1), hits.Load())
		})
	}
}

func TestGenerateRetryableThenSuccess(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error": {"message": "slow down"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"id": "chatcmpl-2",
			"model": "m",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "recovered"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4}
		}`)
	}))
	defer server.Close()

	a := New(Config{
		BaseURL:          server.URL,
		APIKey:           "k",
		ProviderModelID:  "m",
		MaxRetries:       3,
		InitialRetryWait: time.Millisecond,
		MaxRetryWait:     5 * time.Millisecond,
	})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.NoError(t, err)
	require.Len(t, resps, 1)
	require.NotNil(t, resps[0].Content)
	require.Len(t, resps[0].Content.Parts, 1)
	assert.Equal(t, "recovered", resps[0].Content.Parts[0].Text.Text)
	assert.Equal(t, int32(2), hits.Load())
}

func TestGenerateStreamText(t *testing.T) {
	var reqBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		reqBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"c1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		io.WriteString(w, "data: {\"id\":\"c1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\n")
		io.WriteString(w, "data: {\"id\":\"c1\",\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)

	assert.Contains(t, string(reqBody), `"stream":true`)
	assert.Contains(t, string(reqBody), `"stream_options":{"include_usage":true}`)

	require.Len(t, resps, 3)

	assert.True(t, resps[0].Partial)
	require.Len(t, resps[0].Content.Parts, 1)
	assert.Equal(t, "Hello", resps[0].Content.Parts[0].Text.Text)

	assert.True(t, resps[1].Partial)
	require.Len(t, resps[1].Content.Parts, 1)
	assert.Equal(t, " world", resps[1].Content.Parts[0].Text.Text)

	final := resps[2]
	assert.True(t, final.TurnComplete)
	assert.False(t, final.Partial)
	require.NotNil(t, final.Content)
	require.Len(t, final.Content.Parts, 1)
	require.NotNil(t, final.Content.Parts[0].Text)
	assert.Equal(t, "Hello world", final.Content.Parts[0].Text.Text)
	require.NotNil(t, final.Usage)
	assert.Equal(t, 10, final.Usage.PromptTokens)
	assert.Equal(t, 2, final.Usage.CompletionTokens)
	assert.Equal(t, 12, final.Usage.TotalTokens)
	assert.Equal(t, model.FinishReasonStop, final.FinishReason)
	assert.Equal(t, "m", final.ModelVersion)
}

func TestGenerateStreamFinalContainsTextAndToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"reasoning_content":"thinking"}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"Let me check."}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)

	require.Len(t, resps, 3)
	final := resps[2]
	assert.True(t, final.TurnComplete)
	require.NotNil(t, final.Content)
	require.Len(t, final.Content.Parts, 3)
	assert.Equal(t, "thinking", final.Content.Parts[0].Thinking.Text)
	assert.Equal(t, "Let me check.", final.Content.Parts[1].Text.Text)
	require.NotNil(t, final.Content.Parts[2].FunctionCall)
	assert.Equal(t, "call_1", final.Content.Parts[2].FunctionCall.ID)
	assert.Equal(t, model.FinishReasonToolCall, final.FinishReason)
}

func TestGenerateStreamToolCallsReversedIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"second","arguments":"{\"b\":"}}]}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"first","arguments":"{\"a\":"}}]}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}}]}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)

	require.Len(t, resps, 1)
	final := resps[0]
	assert.True(t, final.TurnComplete)
	require.NotNil(t, final.Content)
	require.Len(t, final.Content.Parts, 2)

	first := final.Content.Parts[0].FunctionCall
	require.NotNil(t, first)
	assert.Equal(t, "call_1", first.ID)
	assert.Equal(t, "first", first.Name)
	assert.Equal(t, map[string]any{"a": float64(1)}, first.Args)

	second := final.Content.Parts[1].FunctionCall
	require.NotNil(t, second)
	assert.Equal(t, "call_2", second.ID)
	assert.Equal(t, "second", second.Name)
	assert.Equal(t, map[string]any{"b": float64(2)}, second.Args)
}

func TestCustomHTTPClientHonored(t *testing.T) {
	var called atomic.Bool
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			called.Store(true)
			assert.Equal(t, "/chat/completions", r.URL.Path)
			assert.Equal(t, "Bearer k", r.Header.Get("Authorization"))
			body := `{
				"id": "chatcmpl-3",
				"model": "m",
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "via custom client"}, "finish_reason": "stop"}],
				"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		}),
	}

	a := New(Config{BaseURL: "https://example.invalid", APIKey: "k", ProviderModelID: "m", HTTPClient: client})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.NoError(t, err)
	require.True(t, called.Load())
	require.Len(t, resps, 1)
	require.NotNil(t, resps[0].Content)
	require.Len(t, resps[0].Content.Parts, 1)
	assert.Equal(t, "via custom client", resps[0].Content.Parts[0].Text.Text)
}

func requireStreamErrorKind(t *testing.T, err error, kind model.StreamErrorKind) *model.StreamError {
	t.Helper()
	var streamErr *model.StreamError
	require.ErrorAs(t, err, &streamErr)
	assert.Equal(t, kind, streamErr.Kind)
	return streamErr
}

func TestGenerateStreamEarlyEOFIsIncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"partial"}}]}`+"\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.Len(t, resps, 1)
	streamErr := requireStreamErrorKind(t, err, model.StreamErrorIncomplete)
	assert.True(t, streamErr.Interrupted)
}

func TestGenerateStreamMalformedChunkIsVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: not-json\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	streamErr := requireStreamErrorKind(t, err, model.StreamErrorMalformed)
	assert.False(t, streamErr.Interrupted)
}

func TestGenerateStreamRejectsIncompleteToolArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":"}}]}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	requireStreamErrorKind(t, err, model.StreamErrorInvalidToolCall)
}

func TestGenerateStreamRejectsMissingToolID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"lookup","arguments":"{}"}}]}}]}`+"\n\n")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	requireStreamErrorKind(t, err, model.StreamErrorInvalidToolCall)
}

func TestGenerateStreamAcceptsFinishReasonWithoutDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`+"\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)
	require.Len(t, resps, 2)
	assert.Equal(t, "done", resps[1].Content.Parts[0].Text.Text)
}

func TestGenerateStreamAcceptsDoneWithoutFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"done"}}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)
	require.Len(t, resps, 2)
	assert.Equal(t, "done", resps[1].Content.Parts[0].Text.Text)
}

func TestGenerateStreamParsesTerminalEventWithoutTrailingBlankLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)
	require.Len(t, resps, 2)
	assert.Equal(t, "done", resps[1].Content.Parts[0].Text.Text)
}

func TestGenerateStreamEnforcesLimits(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"`+strings.Repeat("x", 200)+`"}}]}`+"\n\n")
		}))
		defer server.Close()

		a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", MaxStreamBytes: 64})
		_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
		requireStreamErrorKind(t, err, model.StreamErrorLimit)
	})

	t.Run("tool calls", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"a","arguments":"{}"}}]}}]}`+"\n\n")
			io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"b","arguments":"{}"}}]}}]}`+"\n\n")
			io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
		}))
		defer server.Close()

		a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", MaxStreamToolCalls: 1})
		_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
		requireStreamErrorKind(t, err, model.StreamErrorLimit)
	})
}

func TestDisableRedirectsRefusesRedirect(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", DisableRedirects: true})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	var providerErr *model.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusFound, providerErr.StatusCode)
	assert.Equal(t, int32(0), targetHits.Load())
}

func TestBoundsSuccessfulResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"`+strings.Repeat("x", 200)+`"}}]}`)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", MaxResponseBytes: 64})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.Error(t, err)
}

func TestProviderErrorDoesNotExposeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"secret-marker"}}`)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-marker")
	var providerErr *model.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, "invalid_request_error", providerErr.Code)
}

func TestStreamIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"partial"}}]}`+"\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", StreamIdleTimeout: 20 * time.Millisecond})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idle timeout")
}

func TestBeforeAttemptGateRunsPerRetry(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	var attempts []int
	a := New(Config{
		BaseURL:          server.URL,
		APIKey:           "k",
		ProviderModelID:  "m",
		MaxRetries:       3,
		InitialRetryWait: time.Millisecond,
		MaxRetryWait:     time.Millisecond,
		BeforeAttempt: func(_ context.Context, attempt int) error {
			attempts = append(attempts, attempt)
			if attempt == 1 {
				return assert.AnError
			}
			return nil
		},
	})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, []int{0, 1}, attempts)
	assert.Equal(t, int32(1), hits.Load())
}

func TestCheckRedirectValidatesDestination(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	a := New(Config{
		BaseURL: server.URL, APIKey: "k", ProviderModelID: "m",
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("unexpected redirect destination")
		},
	})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected redirect destination")
	assert.Equal(t, int32(0), targetHits.Load())
}
