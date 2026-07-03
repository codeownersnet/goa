package provider

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/modelsdev"
)

var (
	ErrModelNotFound = fmt.Errorf("model not found in registry")
	ErrNoAPIKey      = fmt.Errorf("no API key found")
	ErrNoFactory     = fmt.Errorf("no adapter factory for provider type")
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]*ProviderInfo
	models    map[string]*ModelEntry
	catalog   *modelsdev.Catalog
	factories map[string]AdapterFactory
}

type RegistryOption func(*registryOptions)

type registryOptions struct {
	offline        bool
	customProvider map[string]*ProviderInfo
	factories      map[string]AdapterFactory
}

func WithRegistryOffline() RegistryOption {
	return func(o *registryOptions) { o.offline = true }
}

func WithCustomProvider(id string, info *ProviderInfo) RegistryOption {
	return func(o *registryOptions) {
		if o.customProvider == nil {
			o.customProvider = make(map[string]*ProviderInfo)
		}
		o.customProvider[id] = info
	}
}

func WithFactory(providerType string, factory AdapterFactory) RegistryOption {
	return func(o *registryOptions) {
		if o.factories == nil {
			o.factories = make(map[string]AdapterFactory)
		}
		o.factories[providerType] = factory
	}
}

func NewRegistry(ctx context.Context, opts ...RegistryOption) (*Registry, error) {
	o := registryOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	r := &Registry{
		providers: make(map[string]*ProviderInfo),
		models:    make(map[string]*ModelEntry),
		factories: make(map[string]AdapterFactory),
	}

	catalogOpts := []modelsdev.CatalogOption{}
	if o.offline {
		catalogOpts = append(catalogOpts, modelsdev.WithOffline())
	}

	catalog, err := modelsdev.LoadCatalog(ctx, catalogOpts...)
	if err != nil {
		return nil, fmt.Errorf("load models.dev catalog: %w", err)
	}
	r.catalog = catalog

	for _, p := range catalog.Providers {
		providerType := modelsdev.InferProviderType(p.ID)
		r.providers[p.ID] = &ProviderInfo{
			ID:      p.ID,
			Name:    p.Name,
			APIBase: p.API,
			EnvVars: p.Env,
			DocURL:  p.Doc,
			Type:    string(providerType),
		}

		for _, m := range p.Models {
			fullName := p.ID + "/" + m.ID
			r.models[fullName] = &ModelEntry{
				FullName:        fullName,
				ProviderID:      p.ID,
				ProviderModelID: m.ID,
				Provider:        r.providers[p.ID],
				Capabilities:    modelsdev.FromModelsDevModel(m),
			}
		}
	}

	for id, info := range o.customProvider {
		r.providers[id] = info
	}

	for pt, factory := range o.factories {
		r.factories[pt] = factory
	}

	return r, nil
}

func (r *Registry) RegisterFactory(providerType string, factory AdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[providerType] = factory
}

func (r *Registry) RegisterProvider(id string, info *ProviderInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[id] = info
}

func (r *Registry) Resolve(ctx context.Context, modelString string, opts ...ProviderOption) (model.Model, error) {
	r.mu.RLock()
	entry, ok := r.models[modelString]
	r.mu.RUnlock()

	if !ok {
		providerID, providerModelID := splitModelString(modelString)
		if providerID != "" {
			r.mu.RLock()
			provider, pok := r.providers[providerID]
			r.mu.RUnlock()

			if pok {
				entry = &ModelEntry{
					FullName:        modelString,
					ProviderID:      providerID,
					ProviderModelID: providerModelID,
					Provider:        provider,
					Capabilities:    model.ModelCapabilities{ToolCall: true},
				}
			}
		}

		if entry == nil {
			return nil, fmt.Errorf("model %q: %w", modelString, ErrModelNotFound)
		}
	}

	r.mu.RLock()
	factory, ok := r.factories[entry.Provider.Type]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider type %q: %w", entry.Provider.Type, ErrNoFactory)
	}

	return factory.NewModel(ctx, entry, opts...)
}

func (r *Registry) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]string, 0, len(r.models))
	for k := range r.models {
		models = append(models, k)
	}
	return models
}

func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]string, 0, len(r.providers))
	for k := range r.providers {
		providers = append(providers, k)
	}
	return providers
}

func splitModelString(s string) (providerID, modelID string) {
	for i, c := range s {
		if c == '/' {
			return s[:i], s[i+1:]
		}
	}
	return "", s
}

func ResolveAPIKey(provider *ProviderInfo) (string, error) {
	for _, envVar := range provider.EnvVars {
		if key := os.Getenv(envVar); key != "" {
			return key, nil
		}
	}
	return "", fmt.Errorf("provider %q: %w (set one of %v)", provider.ID, ErrNoAPIKey, provider.EnvVars)
}
