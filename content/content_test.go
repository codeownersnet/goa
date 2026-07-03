package content

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTextPart(t *testing.T) {
	p := NewTextPart("hello")
	require.NotNil(t, p.Text)
	assert.Equal(t, "hello", p.Text.Text)
	assert.Equal(t, "text", p.Type())
}

func TestNewFunctionCallPart(t *testing.T) {
	args := map[string]any{"location": "SF"}
	p := NewFunctionCallPart("call-1", "get_weather", args)
	require.NotNil(t, p.FunctionCall)
	assert.Equal(t, "call-1", p.FunctionCall.ID)
	assert.Equal(t, "get_weather", p.FunctionCall.Name)
	assert.Equal(t, "SF", p.FunctionCall.Args["location"])
	assert.Equal(t, "function_call", p.Type())
}

func TestNewFunctionResponsePart(t *testing.T) {
	resp := map[string]any{"temp": 72}
	p := NewFunctionResponsePart("call-1", "get_weather", resp, false)
	require.NotNil(t, p.FunctionResponse)
	assert.Equal(t, "call-1", p.FunctionResponse.ID)
	assert.False(t, p.FunctionResponse.IsError)

	pErr := NewFunctionResponsePart("call-2", "get_weather", resp, true)
	assert.True(t, pErr.FunctionResponse.IsError)
}

func TestNewThinkingPart(t *testing.T) {
	p := NewThinkingPart("let me think...")
	require.NotNil(t, p.Thinking)
	assert.Equal(t, "let me think...", p.Thinking.Text)
	assert.Equal(t, "thinking", p.Type())
}

func TestNewContent(t *testing.T) {
	c := NewContent(RoleUser, NewTextPart("hi"), NewTextPart("there"))
	assert.Equal(t, RoleUser, c.Role)
	assert.Len(t, c.Parts, 2)
}

func TestNewTextContent(t *testing.T) {
	c := NewTextContent("hello", RoleModel)
	assert.Equal(t, RoleModel, c.Role)
	assert.Len(t, c.Parts, 1)
	assert.Equal(t, "hello", c.Parts[0].Text.Text)
}

func TestPartTypeUnknown(t *testing.T) {
	p := Part{}
	assert.Equal(t, "unknown", p.Type())
}

func TestRoles(t *testing.T) {
	assert.Equal(t, Role("user"), RoleUser)
	assert.Equal(t, Role("model"), RoleModel)
	assert.Equal(t, Role("system"), RoleSystem)
	assert.Equal(t, Role("tool"), RoleTool)
}
