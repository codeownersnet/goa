package modelsdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const catalogURL = "https://models.dev/api.json"

type CatalogOption func(*catalogOptions)

type catalogOptions struct {
	offline     bool
	httpTimeout time.Duration
	customURL   string
}

func WithOffline() CatalogOption {
	return func(o *catalogOptions) { o.offline = true }
}

func WithHTTPTimeout(d time.Duration) CatalogOption {
	return func(o *catalogOptions) { o.httpTimeout = d }
}

func WithCustomURL(url string) CatalogOption {
	return func(o *catalogOptions) { o.customURL = url }
}

func LoadCatalog(ctx context.Context, opts ...CatalogOption) (*Catalog, error) {
	o := catalogOptions{httpTimeout: 10 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}

	if !o.offline {
		url := catalogURL
		if o.customURL != "" {
			url = o.customURL
		}
		catalog, err := fetchCatalog(ctx, url, o.httpTimeout)
		if err == nil {
			return catalog, nil
		}
	}

	embedded, err := loadEmbeddedFallback()
	if err != nil {
		return nil, fmt.Errorf("load models.dev catalog: no live data and no embedded fallback: %w", err)
	}
	return embedded, nil
}

func fetchCatalog(ctx context.Context, url string, timeout time.Duration) (*Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch catalog: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read catalog response: %w", err)
	}

	return ParseCatalog(data)
}

func ParseCatalog(data []byte) (*Catalog, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse catalog JSON: %w", err)
	}

	var catalog Catalog
	for id, rawProvider := range raw {
		if id == "$schema" || id == "version" {
			continue
		}
		var p Provider
		if err := json.Unmarshal(rawProvider, &p); err != nil {
			continue
		}
		if p.ID == "" {
			p.ID = id
		}
		catalog.Providers = append(catalog.Providers, p)
	}

	return &catalog, nil
}

func loadEmbeddedFallback() (*Catalog, error) {
	paths := []string{
		"catalog.json",
		filepath.Join(os.Getenv("HOME"), ".goa", "catalog.json"),
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return ParseCatalog(data)
	}

	return nil, fmt.Errorf("no embedded catalog found")
}

//go:generate go run ./../cmd/embed-catalog/main.go

// YAML parsing helper for SKILL.md integration
func ParseYAML(data []byte, out any) error {
	return yaml.Unmarshal(data, out)
}
