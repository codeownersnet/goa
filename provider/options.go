package provider

import "github.com/codeownersnet/goa/model"

type ProviderOption func(*providerOptions)

type providerOptions struct {
	APIKey       string
	APIBase      string
	Capabilities *model.ModelCapabilities
}

func WithAPIKey(key string) ProviderOption {
	return func(o *providerOptions) { o.APIKey = key }
}

func WithAPIBase(url string) ProviderOption {
	return func(o *providerOptions) { o.APIBase = url }
}

func WithCapabilities(c model.ModelCapabilities) ProviderOption {
	return func(o *providerOptions) { o.Capabilities = &c }
}

func ApplyOptions(opts ...ProviderOption) *providerOptions {
	o := &providerOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
