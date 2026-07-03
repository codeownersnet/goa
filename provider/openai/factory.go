package openai

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/provider"
)

type Factory struct{}

func (f *Factory) NewModel(_ context.Context, entry *provider.ModelEntry, opts ...provider.ProviderOption) (model.Model, error) {
	o := provider.ApplyOptions(opts...)

	cfg := Config{
		BaseURL:         entry.Provider.APIBase,
		APIKey:          o.APIKey,
		ProviderModelID: entry.ProviderModelID,
	}

	if o.APIBase != "" {
		cfg.BaseURL = o.APIBase
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}

	if cfg.APIKey == "" {
		key, err := provider.ResolveAPIKey(entry.Provider)
		if err != nil {
			return nil, fmt.Errorf("openai factory: %w", err)
		}
		cfg.APIKey = key
	}

	adapter := New(cfg)
	adapter.caps = entry.Capabilities
	return adapter, nil
}

var _ provider.AdapterFactory = (*Factory)(nil)
