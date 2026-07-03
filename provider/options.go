package provider

type ProviderOption func(*providerOptions)

type providerOptions struct {
	APIKey  string
	APIBase string
}

func WithAPIKey(key string) ProviderOption {
	return func(o *providerOptions) { o.APIKey = key }
}

func WithAPIBase(url string) ProviderOption {
	return func(o *providerOptions) { o.APIBase = url }
}

func ApplyOptions(opts ...ProviderOption) *providerOptions {
	o := &providerOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
