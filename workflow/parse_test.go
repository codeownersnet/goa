package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMinimal(t *testing.T) {
	data := []byte(`
name: test
description: A test workflow
steps:
  - name: step1
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do something
`)
	raw, err := parse(data)
	require.NoError(t, err)
	assert.Equal(t, "test", raw.Name)
	assert.Equal(t, "A test workflow", raw.Description)
	assert.Len(t, raw.Steps, 1)
	assert.Equal(t, "step1", raw.Steps[0].Name)
	assert.Equal(t, "synthetic/hf:zai-org/GLM-4.7-Flash", raw.Steps[0].Model)
}

func TestParseLoopWithExitWhen(t *testing.T) {
	data := []byte(`
name: loop-test
description: A loop workflow
type: loop
max_iterations: 3
exit_when:
  state:
    done: "true"
  timeout: 5m
steps:
  - name: work
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Work
`)
	raw, err := parse(data)
	require.NoError(t, err)
	assert.Equal(t, "loop", raw.Type)
	assert.Equal(t, 3, raw.MaxIterations)
	require.NotNil(t, raw.ExitWhen)
	assert.Equal(t, "true", raw.ExitWhen.State["done"])
	assert.Equal(t, "5m", raw.ExitWhen.Timeout)
}

func TestParseNestedSteps(t *testing.T) {
	data := []byte(`
name: nested
description: Nested workflow
steps:
  - name: outer
    type: loop
    max_iterations: 5
    steps:
      - name: inner1
        model: synthetic/hf:zai-org/GLM-4.7-Flash
        instruction: Inner 1
      - name: inner2
        model: synthetic/hf:zai-org/GLM-4.7-Flash
        instruction: Inner 2
`)
	raw, err := parse(data)
	require.NoError(t, err)
	require.Len(t, raw.Steps, 1)
	assert.Equal(t, "loop", raw.Steps[0].Type)
	assert.Len(t, raw.Steps[0].Steps, 2)
}

func TestParseGenerateConfig(t *testing.T) {
	data := []byte(`
name: gen-cfg
description: With generate config
steps:
  - name: step1
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do something
    generate_config:
      temperature: 0.5
      max_tokens: 2048
      tool_choice: auto
      stop_sequences: ["STOP"]
`)
	raw, err := parse(data)
	require.NoError(t, err)
	require.NotNil(t, raw.Steps[0].GenerateConfig)
	assert.InDelta(t, 0.5, *raw.Steps[0].GenerateConfig.Temperature, 0.001)
	assert.Equal(t, 2048, raw.Steps[0].GenerateConfig.MaxTokens)
	assert.Equal(t, "auto", raw.Steps[0].GenerateConfig.ToolChoice)
	assert.Equal(t, []string{"STOP"}, raw.Steps[0].GenerateConfig.StopSequences)
}

func TestParseMalformedYAML(t *testing.T) {
	data := []byte(`{invalid yaml: [`)
	_, err := parse(data)
	assert.Error(t, err)
}

func TestParseSkillsAndTools(t *testing.T) {
	data := []byte(`
name: skill-test
description: With skills
steps:
  - name: step1
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do something
    tools: [bash, read_file, grep]
    skills: [code-review]
    auto_skills: false
`)
	raw, err := parse(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"bash", "read_file", "grep"}, raw.Steps[0].Tools)
	assert.Equal(t, []string{"code-review"}, raw.Steps[0].Skills)
	require.NotNil(t, raw.Steps[0].AutoSkills)
	assert.False(t, *raw.Steps[0].AutoSkills)
}

func TestParseWithMCPServers(t *testing.T) {
	data := []byte(`
name: mcp-test
description: With MCP servers
mcp_servers:
  filesystem:
    command: "npx -y @modelcontextprotocol/server-filesystem /tmp"
  remote-api:
    url: "https://mcp.example.com/sse"
    headers:
      Authorization: "Bearer token"
    connect_timeout: 10s
    tool_timeout: 120s
steps:
  - name: step1
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do something
    tools: [mcp:filesystem, bash]
`)
	raw, err := parse(data)
	require.NoError(t, err)
	require.Len(t, raw.MCPServers, 2)
	assert.Equal(t, "npx -y @modelcontextprotocol/server-filesystem /tmp", raw.MCPServers["filesystem"].Command)
	assert.Equal(t, "https://mcp.example.com/sse", raw.MCPServers["remote-api"].URL)
	assert.Equal(t, "Bearer token", raw.MCPServers["remote-api"].Headers["Authorization"])
	assert.Equal(t, "10s", raw.MCPServers["remote-api"].ConnectTimeout)
	assert.Equal(t, "120s", raw.MCPServers["remote-api"].ToolTimeout)
}

func TestParseWithoutMCPServers(t *testing.T) {
	data := []byte(`
name: no-mcp
description: No MCP
steps:
  - name: step1
    model: m
    instruction: i
`)
	raw, err := parse(data)
	require.NoError(t, err)
	assert.Nil(t, raw.MCPServers)
}

func TestParseWithProviders(t *testing.T) {
	data := []byte(`
name: prov-test
description: With custom providers
providers:
  litellm:
    api_base: "http://localhost:4000/v1"
    env: ["LITELLM_API_KEY"]
    type: openai_compat
  ollama:
    api_base: "http://localhost:11434/v1"
steps:
  - name: step1
    model: litellm/gpt-4o
    instruction: Do something
`)
	raw, err := parse(data)
	require.NoError(t, err)
	require.Len(t, raw.Providers, 2)

	litellm := raw.Providers["litellm"]
	assert.Equal(t, "http://localhost:4000/v1", litellm.APIBase)
	assert.Equal(t, []string{"LITELLM_API_KEY"}, litellm.Env)
	assert.Equal(t, "openai_compat", litellm.Type)

	ollama := raw.Providers["ollama"]
	assert.Equal(t, "http://localhost:11434/v1", ollama.APIBase)
	assert.Empty(t, ollama.Env)
	assert.Empty(t, ollama.Type)
}

func TestParseWithoutProviders(t *testing.T) {
	data := []byte(`
name: no-prov
description: No providers
steps:
  - name: step1
    model: m
    instruction: i
`)
	raw, err := parse(data)
	require.NoError(t, err)
	assert.Nil(t, raw.Providers)
}
