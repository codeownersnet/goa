package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type AuthConfig struct {
	APIKey       string
	AuthType     string
	ExtraHeaders map[string]string
}

func NewHTTPRequest(ctx context.Context, auth AuthConfig, baseURL, method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := strings.TrimRight(baseURL, "/") + path

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	switch auth.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+auth.APIKey)
	case "x-api-key":
		req.Header.Set("x-api-key", auth.APIKey)
	}

	for k, v := range auth.ExtraHeaders {
		req.Header.Set(k, v)
	}

	return req, nil
}

func NewHTTPClient(base *http.Client, disableRedirects bool, checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	client := &http.Client{}
	if base != nil {
		*client = *base
	}
	switch {
	case disableRedirects:
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	case checkRedirect != nil:
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			err := checkRedirect(req, via)
			if err == nil || errors.Is(err, http.ErrUseLastResponse) {
				return err
			}
			return &RedirectError{Err: err}
		}
	}
	return client
}

type RedirectError struct {
	Err error
}

func (e *RedirectError) Error() string {
	return "redirect rejected: " + e.Err.Error()
}

func (e *RedirectError) Unwrap() error {
	return e.Err
}

type ErrorInfo struct {
	Code string
}

func ReadErrorInfo(r io.Reader, maxBytes int) ErrorInfo {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}
	data, err := io.ReadAll(io.LimitReader(r, int64(maxBytes)))
	if err != nil {
		return ErrorInfo{}
	}
	var envelope struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ErrorInfo{}
	}
	code := envelope.Error.Code
	if code == "" {
		code = envelope.Error.Type
	}
	return ErrorInfo{Code: code}
}

func ProviderRequestID(header http.Header) string {
	for _, name := range []string{"x-request-id", "request-id", "x-amzn-requestid", "openai-request-id"} {
		if value := header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

type idleTimeoutReader struct {
	reader   io.ReadCloser
	timeout  time.Duration
	timer    *time.Timer
	timedOut atomic.Bool
}

func NewIdleTimeoutReader(r io.ReadCloser, timeout time.Duration) io.ReadCloser {
	if timeout <= 0 {
		return r
	}
	return &idleTimeoutReader{reader: r, timeout: timeout}
}

func (r *idleTimeoutReader) Read(p []byte) (int, error) {
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer = time.AfterFunc(r.timeout, func() {
		r.timedOut.Store(true)
		_ = r.reader.Close()
	})
	n, err := r.reader.Read(p)
	if r.timer != nil {
		r.timer.Stop()
	}
	if err != nil && r.timedOut.Load() {
		return n, fmt.Errorf("stream idle timeout after %s: %w", r.timeout, err)
	}
	return n, err
}

func (r *idleTimeoutReader) Close() error {
	if r.timer != nil {
		r.timer.Stop()
	}
	return r.reader.Close()
}
