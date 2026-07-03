package model

import "github.com/codeownersnet/goa/schema"

type GenerateConfig struct {
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	MaxTokens         int             `json:"max_tokens,omitempty"`
	StopSequences     []string        `json:"stop_sequences,omitempty"`
	ToolChoice        ToolChoice      `json:"tool_choice,omitempty"`
	ResponseSchema    *schema.Schema  `json:"response_schema,omitempty"`
	ResponseMIMEType  string          `json:"response_mime_type,omitempty"`
	SystemInstruction string          `json:"system_instruction,omitempty"`
	ThinkingConfig    *ThinkingConfig `json:"thinking_config,omitempty"`
	SafetySettings    []SafetySetting `json:"safety_settings,omitempty"`
	ProviderExtra     map[string]any  `json:"provider_extra,omitempty"`
}

type ToolChoice struct {
	Mode ToolChoiceMode `json:"mode"`
	Name string         `json:"name,omitempty"`
}

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceSpecific ToolChoiceMode = "specific"
)

type ThinkingConfig struct {
	Enabled         bool `json:"enabled"`
	BudgetTokens    int  `json:"budget_tokens,omitempty"`
	IncludeThoughts bool `json:"include_thoughts,omitempty"`
}

type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}
