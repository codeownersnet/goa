package anthropic

import (
	"context"
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
		assert.Equal(t, "/messages", r.URL.Path)
		assert.Equal(t, "k", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "m",
			"content": [{"type": "text", "text": "Hi there"}],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 12, "output_tokens": 7, "cache_read_input_tokens": 3}
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
	assert.Equal(t, "Hi there", resp.Content.Parts[0].Text.Text)

	require.NotNil(t, resp.Usage)
	assert.Equal(t, 12, resp.Usage.PromptTokens)
	assert.Equal(t, 7, resp.Usage.CompletionTokens)
	assert.Equal(t, 3, resp.Usage.CacheReadTokens)

	assert.Equal(t, model.FinishReasonStop, resp.FinishReason)
	assert.Equal(t, "m", resp.ModelVersion)
	assert.Equal(t, int32(1), hits.Load())
}

func TestGenerateStreamText(t *testing.T) {
	var reqBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		reqBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message_start\n")
		io.WriteString(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"usage":{"input_tokens":25,"output_tokens":1}}}`+"\n\n")
		io.WriteString(w, "event: content_block_start\n")
		io.WriteString(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\n")
		io.WriteString(w, "event: content_block_delta\n")
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`+"\n\n")
		io.WriteString(w, "event: content_block_delta\n")
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`+"\n\n")
		io.WriteString(w, "event: message_delta\n")
		io.WriteString(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`+"\n\n")
		io.WriteString(w, "event: message_stop\n")
		io.WriteString(w, `data: {"type":"message_stop"}`+"\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)

	assert.Contains(t, string(reqBody), `"stream":true`)

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
	assert.Equal(t, model.FinishReasonStop, final.FinishReason)
	require.NotNil(t, final.Content)
	require.Len(t, final.Content.Parts, 1)
	require.NotNil(t, final.Content.Parts[0].Text)
	assert.Equal(t, "Hello world", final.Content.Parts[0].Text.Text)

	require.NotNil(t, final.Usage)
	assert.Equal(t, 12, final.Usage.CompletionTokens)
	assert.Equal(t, 25, final.Usage.PromptTokens)
}

func TestGenerateSingleAttemptDoesNotRetry(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, `{"type": "error", "error": {"type": "overloaded_error", "message": "overloaded"}}`)
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
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.Error(t, err)
	assert.Equal(t, int32(1), hits.Load())
}

func TestGenerateUnauthorizedError(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type": "error", "error": {"type": "authentication_error", "message": "invalid x-api-key"}}`)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.Error(t, err)
	var providerErr *model.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusUnauthorized, providerErr.StatusCode)
	assert.Equal(t, "authentication_error", providerErr.Code)
	assert.NotContains(t, err.Error(), "invalid x-api-key")
	assert.Equal(t, int32(1), hits.Load())
}

func requireAnthropicStreamErrorKind(t *testing.T, err error, kind model.StreamErrorKind) *model.StreamError {
	t.Helper()
	var streamErr *model.StreamError
	require.ErrorAs(t, err, &streamErr)
	assert.Equal(t, kind, streamErr.Kind)
	return streamErr
}

func TestGenerateStreamEarlyEOFIsIncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`+"\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.Len(t, resps, 1)
	streamErr := requireAnthropicStreamErrorKind(t, err, model.StreamErrorIncomplete)
	assert.True(t, streamErr.Interrupted)
}

func TestGenerateStreamMalformedEventIsVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: not-json\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	streamErr := requireAnthropicStreamErrorKind(t, err, model.StreamErrorMalformed)
	assert.False(t, streamErr.Interrupted)
}

func TestGenerateStreamRejectsIncompleteToolArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup"}}`+"\n\n")
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`+"\n\n")
		io.WriteString(w, `data: {"type":"content_block_stop","index":0}`+"\n\n")
		io.WriteString(w, `data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}`+"\n\n")
		io.WriteString(w, `data: {"type":"message_stop"}`+"\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	requireAnthropicStreamErrorKind(t, err, model.StreamErrorInvalidToolCall)
}

func TestGenerateStreamRejectsUnfinishedToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup"}}`+"\n\n")
		io.WriteString(w, `data: {"type":"message_stop"}`+"\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	requireAnthropicStreamErrorKind(t, err, model.StreamErrorInvalidToolCall)
}

func TestGenerateStreamAcceptsStopReasonWithoutMessageStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`+"\n\n")
		io.WriteString(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`+"\n\n")
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m"})
	resps, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
	require.NoError(t, err)
	require.Len(t, resps, 2)
	assert.Equal(t, "done", resps[1].Content.Parts[0].Text.Text)
}

func TestGenerateStreamParsesMessageStopWithoutTrailingBlankLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`+"\n\n")
		io.WriteString(w, `data: {"type":"message_stop"}`)
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
			io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"`+strings.Repeat("x", 200)+`"}}`+"\n\n")
		}))
		defer server.Close()

		a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", MaxStreamBytes: 64})
		_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
		requireAnthropicStreamErrorKind(t, err, model.StreamErrorLimit)
	})

	t.Run("tool calls", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"a"}}`+"\n\n")
			io.WriteString(w, `data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_2","name":"b"}}`+"\n\n")
		}))
		defer server.Close()

		a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", MaxStreamToolCalls: 1})
		_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), true))
		requireAnthropicStreamErrorKind(t, err, model.StreamErrorLimit)
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

func TestProviderErrorDoesNotExposeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"secret-marker"}}`)
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
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`+"\n\n")
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

func TestBoundsSuccessfulResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"`+strings.Repeat("x", 200)+`"}]}`)
	}))
	defer server.Close()

	a := New(Config{BaseURL: server.URL, APIKey: "k", ProviderModelID: "m", MaxResponseBytes: 64})
	_, err := collectStream(a.GenerateContent(context.Background(), userRequest(), false))
	require.Error(t, err)
}
