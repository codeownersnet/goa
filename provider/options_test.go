package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterProvider(t *testing.T) {
	r := &Registry{
		providers: make(map[string]*ProviderInfo),
		models:    make(map[string]*ModelEntry),
		factories: make(map[string]AdapterFactory),
	}

	info := &ProviderInfo{
		ID:      "litellm",
		Name:    "LiteLLM",
		APIBase: "http://localhost:4000/v1",
		EnvVars: []string{"LITELLM_API_KEY"},
		Type:    "openai_compat",
	}

	r.RegisterProvider("litellm", info)

	assert.Equal(t, info, r.providers["litellm"])
}

func TestWithAPIBase(t *testing.T) {
	o := ApplyOptions(WithAPIBase("http://localhost:4000/v1"))
	assert.Equal(t, "http://localhost:4000/v1", o.APIBase)
}

func TestWithAPIKey(t *testing.T) {
	o := ApplyOptions(WithAPIKey("sk-test"))
	assert.Equal(t, "sk-test", o.APIKey)
	assert.Empty(t, o.APIBase)
}

func TestApplyOptionsDefaults(t *testing.T) {
	o := ApplyOptions()
	assert.Empty(t, o.APIKey)
	assert.Empty(t, o.APIBase)
}
