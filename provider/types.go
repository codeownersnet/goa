package provider

import (
	"context"

	"github.com/codeownersnet/goa/model"
)

type ProviderInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	APIBase string   `json:"api_base,omitempty"`
	EnvVars []string `json:"env"`
	DocURL  string   `json:"doc_url,omitempty"`
	Type    string   `json:"type"`
}

type ModelEntry struct {
	FullName        string                  `json:"full_name"`
	ProviderID      string                  `json:"provider_id"`
	ProviderModelID string                  `json:"provider_model_id"`
	Provider        *ProviderInfo           `json:"provider"`
	Capabilities    model.ModelCapabilities `json:"capabilities"`
}

type AdapterFactory interface {
	NewModel(ctx context.Context, entry *ModelEntry, opts ...ProviderOption) (model.Model, error)
}
