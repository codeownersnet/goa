package modelsdev

import (
	"context"
	_ "embed"
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

//go:embed catalog.json
var embeddedCatalog []byte

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
		catalog, body, err := fetchCatalog(ctx, url, o.httpTimeout)
		if err == nil {
			writeCatalogCache(body)
			return catalog, nil
		}
	}

	embedded, err := loadEmbeddedFallback()
	if err != nil {
		return nil, fmt.Errorf("load models.dev catalog: no live data and no embedded fallback: %w", err)
	}
	return embedded, nil
}

func fetchCatalog(ctx context.Context, url string, timeout time.Duration) (*Catalog, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetch catalog: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read catalog response: %w", err)
	}

	catalog, err := ParseCatalog(data)
	if err != nil {
		return nil, nil, err
	}
	return catalog, data, nil
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
	if home, err := os.UserHomeDir(); err == nil {
		data, err := os.ReadFile(filepath.Join(home, ".goa", "catalog.json"))
		if err == nil {
			return ParseCatalog(data)
		}
	}

	if len(embeddedCatalog) > 0 {
		return ParseCatalog(embeddedCatalog)
	}

	return nil, fmt.Errorf("no embedded catalog found")
}

func writeCatalogCache(body []byte) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".goa")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "catalog.json")
	_ = os.WriteFile(path, body, 0o644)
}

//go:generate go run ./../cmd/embed-catalog

// YAML parsing helper for SKILL.md integration
func ParseYAML(data []byte, out any) error {
	return yaml.Unmarshal(data, out)
}
