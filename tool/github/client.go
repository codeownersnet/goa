package github

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/go-github/v88/github"
)

type Config struct {
	Token      string
	Timeout    int
	HTTPClient *http.Client
}

func resolveToken(cfg Config) (string, error) {
	if cfg.Token != "" {
		return cfg.Token, nil
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("github tool: GITHUB_TOKEN environment variable or Config.Token must be set")
}

type clientResult struct {
	client   *github.Client
	tokenErr error
}

func newClientWithOptions(cfg Config, opts ...github.ClientOptionsFunc) (clientResult, error) {
	token, tokenErr := resolveToken(cfg)

	allOpts := []github.ClientOptionsFunc{}
	if token != "" {
		allOpts = append(allOpts, github.WithAuthToken(token))
	}
	if cfg.HTTPClient != nil {
		allOpts = append(allOpts, github.WithHTTPClient(cfg.HTTPClient))
	} else {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30
		}
		allOpts = append(allOpts, github.WithTimeout(time.Duration(timeout)*time.Second))
	}
	allOpts = append(allOpts, opts...)

	client, err := github.NewClient(allOpts...)
	if err != nil {
		return clientResult{}, err
	}

	return clientResult{client: client, tokenErr: tokenErr}, nil
}
