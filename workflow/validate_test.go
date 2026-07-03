package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateMissingName(t *testing.T) {
	raw := &workflowYAML{
		Description: "desc",
		Steps:       []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestValidateMissingDescription(t *testing.T) {
	raw := &workflowYAML{
		Name:  "test",
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "description")
}

func TestValidateEmptySteps(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc", Steps: []stepYAML{},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "steps")
}

func TestValidateInvalidType(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc", Type: "parallel",
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type")
}

func TestValidateLLMStepMissingModel(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model")
}

func TestValidateLLMStepMissingInstruction(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "instruction")
}

func TestValidateLoopStepMissingSubSteps(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLoop}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "steps")
}

func TestValidateInvalidToolChoice(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Steps: []stepYAML{{
			Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i",
			GenerateConfig: &generateConfigYAML{ToolChoice: "invalid"},
		}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool_choice")
}

func TestValidateInvalidTimeout(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		ExitWhen: &exitWhenYAML{Timeout: "not-a-duration"},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestValidateDuplicateStepNames(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Steps: []stepYAML{
			{Name: "dup", Type: stepTypeLLM, Model: "m", Instruction: "i1"},
			{Name: "dup", Type: stepTypeLLM, Model: "m", Instruction: "i2"},
		},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidateValidMinimal(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.NoError(t, err)
	assert.Equal(t, wfTypeSequential, raw.Type)
}

func TestValidateValidLoopWithExitWhen(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc", Type: wfTypeLoop, MaxIterations: 5,
		ExitWhen: &exitWhenYAML{State: map[string]string{"done": "true"}, Timeout: "5m"},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.NoError(t, err)
}

func TestValidateDefaultStepType(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Steps: []stepYAML{{Name: "s1", Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.NoError(t, err)
	assert.Equal(t, stepTypeLLM, raw.Steps[0].Type)
}

func TestValidateMCPServersValid(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		MCPServers: map[string]mcpServerYAML{
			"fs": {Command: "npx -y @mcp/server-filesystem /tmp"},
			"api": {URL: "https://mcp.example.com/sse", Headers: map[string]string{"Auth": "Bearer x"}, ConnectTimeout: "10s", ToolTimeout: "120s"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.NoError(t, err)
}

func TestValidateMCPServersInvalidName(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		MCPServers: map[string]mcpServerYAML{
			"123bad": {Command: "echo"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mcp_servers.123bad")
}

func TestValidateMCPServersNeitherCommandNorURL(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		MCPServers: map[string]mcpServerYAML{
			"srv": {},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must specify command or url")
}

func TestValidateMCPServersBothCommandAndURL(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		MCPServers: map[string]mcpServerYAML{
			"srv": {Command: "echo", URL: "https://example.com"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must specify command or url, not both")
}

func TestValidateMCPServersBadConnectTimeout(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		MCPServers: map[string]mcpServerYAML{
			"srv": {URL: "https://example.com", ConnectTimeout: "bad"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connect_timeout")
}

func TestValidateMCPServersBadToolTimeout(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		MCPServers: map[string]mcpServerYAML{
			"srv": {URL: "https://example.com", ToolTimeout: "bad"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool_timeout")
}

func TestValidateProvidersValid(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Providers: map[string]providerYAML{
			"litellm": {APIBase: "http://localhost:4000/v1", Env: []string{"LITELLM_API_KEY"}, Type: "openai_compat"},
			"ollama":  {APIBase: "http://localhost:11434/v1"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.NoError(t, err)
}

func TestValidateProvidersInvalidName(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Providers: map[string]providerYAML{
			"123bad": {APIBase: "http://localhost:4000/v1"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "providers.123bad")
}

func TestValidateProvidersMissingAPIBase(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Providers: map[string]providerYAML{
			"myprov": {APIBase: ""},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_base")
}

func TestValidateProvidersInvalidType(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Providers: map[string]providerYAML{
			"myprov": {APIBase: "http://localhost:4000/v1", Type: "invalid_type"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type")
}

func TestValidateProvidersAnthropicType(t *testing.T) {
	raw := &workflowYAML{
		Name: "test", Description: "desc",
		Providers: map[string]providerYAML{
			"myanth": {APIBase: "http://localhost:4000/v1", Type: "anthropic"},
		},
		Steps: []stepYAML{{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i"}},
	}
	err := validate(raw)
	assert.NoError(t, err)
}
