package model

type ModelCapabilities struct {
	ToolCall         bool            `json:"tool_call"`
	StructuredOutput bool            `json:"structured_output"`
	Reasoning        bool            `json:"reasoning"`
	Attachment       bool            `json:"attachment"`
	InputModalities  map[string]bool `json:"input_modalities"`
	OutputModalities map[string]bool `json:"output_modalities"`
	ContextLimit     int             `json:"context_limit"`
	OutputLimit      int             `json:"output_limit"`
}
