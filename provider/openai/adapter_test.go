package openai

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
	assert.Contains(t, err.Error(), "HTTP 400")
	assert.Contains(t, err.Error(), "invalid request")
	assert.Equal(t, int32(1), hits.Load())
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
	require.NotNil(t, final.Usage)
	assert.Equal(t, 10, final.Usage.PromptTokens)
	assert.Equal(t, 2, final.Usage.CompletionTokens)
	assert.Equal(t, 12, final.Usage.TotalTokens)
	assert.Equal(t, model.FinishReasonStop, final.FinishReason)
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
