package mcptoolset

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type transportConfig struct {
	command    *exec.Cmd
	url        string
	headers    map[string]string
	env        []string
	timeout    time.Duration
	httpClient *http.Client
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
	httpClient := cfg.httpClient
	if httpClient == nil {
		if cfg.timeout > 0 {
			httpClient = &http.Client{
				Timeout: cfg.timeout,
			}
		} else {
			httpClient = http.DefaultClient
		}
	}

	return &mcp.StreamableClientTransport{
		Endpoint:   cfg.url,
		HTTPClient: httpClient,
	}, nil
}
