package workflow

import (
	"context"
	"testing"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/loopagent"
	"github.com/codeownersnet/goa/agent/sequentialagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	toolregistry "github.com/codeownersnet/goa/tool/registry"
)

func TestLoadSequentialFromBytes(t *testing.T) {
	data := []byte(`
name: seq-test
description: Sequential test workflow
steps:
  - name: step1
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do step 1
    tools: [bash, read_file]
  - name: step2
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do step 2
    tools: [grep]
`)

	toolReg := toolregistry.DefaultBuiltinRegistry()
	_, err := LoadFromBytes(context.Background(), data,
		WithToolRegistry(toolReg),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider registry")
}

func TestLoadFromBytesValidation(t *testing.T) {
	data := []byte(`
name: ""
description: ""
steps: []
`)
	_, err := LoadFromBytes(context.Background(), data)
	assert.Error(t, err)
}

func TestValidateOnlyFromBytes(t *testing.T) {
	data := []byte(`
name: test
description: test workflow
steps:
  - name: step1
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do something
    tools: [bash]
    exit_when:
      state:
        done: "true"
      timeout: 5m
`)
	toolReg := toolregistry.DefaultBuiltinRegistry()
	_, err := LoadFromBytes(context.Background(), data,
		WithToolRegistry(toolReg),
	)
	require.Error(t, err)
}

func TestLoadWithExitWhen(t *testing.T) {
	data := []byte(`
name: exit-test
description: Test exit condition
type: loop
max_iterations: 3
exit_when:
  state:
    status: complete
  timeout: 10m
steps:
  - name: work
    model: synthetic/hf:zai-org/GLM-4.7-Flash
    instruction: Do work
    tools: [exit_loop]
`)
	toolReg := toolregistry.DefaultBuiltinRegistry()
	_, err := LoadFromBytes(context.Background(), data,
		WithToolRegistry(toolReg),
	)
	require.Error(t, err)
}

func TestAgentTreeStructure(t *testing.T) {
	raw := &workflowYAML{
		Name:        "tree-test",
		Description: "Tree structure test",
		Type:        wfTypeSequential,
		Steps: []stepYAML{
			{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i1", Tools: []string{"bash"}},
			{Name: "s2", Type: stepTypeLoop, MaxIterations: 3, Steps: []stepYAML{
				{Name: "inner", Type: stepTypeLLM, Model: "m", Instruction: "i2", Tools: []string{"exit_loop"}},
			}},
		},
	}

	err := validate(raw)
	require.NoError(t, err)

	assert.Equal(t, "tree-test", raw.Name)
	assert.Len(t, raw.Steps, 2)
	assert.Equal(t, stepTypeLLM, raw.Steps[0].Type)
	assert.Equal(t, stepTypeLoop, raw.Steps[1].Type)
	assert.Len(t, raw.Steps[1].Steps, 1)
}

func TestWorkflowAgentType(t *testing.T) {
	raw := &workflowYAML{
		Name:          "type-test",
		Description:   "Type test",
		Type:          wfTypeLoop,
		MaxIterations: 5,
		Steps: []stepYAML{
			{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i1"},
		},
	}

	err := validate(raw)
	require.NoError(t, err)
	assert.Equal(t, wfTypeLoop, raw.Type)
}

func TestResolveSequentialTree(t *testing.T) {
	raw := &workflowYAML{
		Name:        "resolve-seq",
		Description: "Resolve sequential",
		Steps: []stepYAML{
			{Name: "s1", Type: stepTypeLLM, Model: "m", Instruction: "i1"},
			{Name: "s2", Type: stepTypeLLM, Model: "m", Instruction: "i2"},
		},
	}

	toolReg := toolregistry.DefaultBuiltinRegistry()

	ag, exitCond, _, err := resolve(context.Background(), raw, resolverConfig{
		toolRegistry: toolReg,
	})
	require.Error(t, err)
	assert.Nil(t, ag)
	assert.Nil(t, exitCond)
}

func TestResolveWithProvider(t *testing.T) {
	raw := &workflowYAML{
		Name:        "resolve-prov",
		Description: "Resolve with provider",
		Steps: []stepYAML{
			{Name: "s1", Type: stepTypeLLM, Model: "nonexistent/model", Instruction: "i1", Tools: []string{"bash"}},
		},
	}

	toolReg := toolregistry.DefaultBuiltinRegistry()

	ag, exitCond, _, err := resolve(context.Background(), raw, resolverConfig{
		toolRegistry: toolReg,
	})
	require.Error(t, err)
	assert.Nil(t, ag)
	assert.Nil(t, exitCond)
}

func TestResolveUnknownTool(t *testing.T) {
	toolReg := toolregistry.DefaultBuiltinRegistry()
	_, err := toolReg.Lookup("nonexistent_tool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent_tool")
}

var _ agent.Agent = (*sequentialagent.Agent)(nil)
var _ agent.Agent = (*loopagent.Agent)(nil)
