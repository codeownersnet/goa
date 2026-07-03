package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
