package mcptoolset

import (
	"fmt"
	"net/http"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type transportConfig struct {
	command              *exec.Cmd
	url                  string
	headers              map[string]string
	env                  []string
	httpClient           *http.Client
	maxEventSize         int
	maxRetries           int
	disableStandaloneSSE bool
}

func newTransport(cfg *transportConfig) (mcp.Transport, error) {
	if cfg.command != nil {
		if len(cfg.env) > 0 {
			cfg.command.Env = append(cfg.command.Environ(), cfg.env...)
		}
		return &mcp.CommandTransport{Command: cfg.command}, nil
	}

	if cfg.url != "" {
		return newHTTPTransport(cfg)
	}

	return nil, fmt.Errorf("mcptoolset: either command or url must be provided")
}

func newHTTPTransport(cfg *transportConfig) (mcp.Transport, error) {
	httpClient := &http.Client{}
	if cfg.httpClient != nil {
		clientCopy := *cfg.httpClient
		httpClient = &clientCopy
	}

	if len(cfg.headers) > 0 {
		base := httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		httpClient.Transport = &headerRoundTripper{
			base:    base,
			headers: cfg.headers,
		}
	}

	maxEventSize := cfg.maxEventSize
	if maxEventSize == 0 {
		maxEventSize = mcp.DefaultMaxEventSize
	}

	return &mcp.StreamableClientTransport{
		Endpoint:             cfg.url,
		HTTPClient:           httpClient,
		MaxEventSize:         maxEventSize,
		MaxRetries:           cfg.maxRetries,
		DisableStandaloneSSE: cfg.disableStandaloneSSE,
	}, nil
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for key, value := range h.headers {
		clone.Header.Set(key, value)
	}
	return h.base.RoundTrip(clone)
}
