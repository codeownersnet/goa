package modelsdev

import "github.com/codeownersnet/goa/model"

func InferProviderType(providerID string) ProviderType {
	switch providerID {
	case "anthropic", "kimi-for-coding":
		return ProviderTypeAnthropic
	default:
		return ProviderTypeOpenAI
	}
}

type ProviderType string

const (
	ProviderTypeOpenAI    ProviderType = "openai_compat"
	ProviderTypeAnthropic ProviderType = "anthropic"
)

func FromModelsDevModel(m Model) model.ModelCapabilities {
	inputMods := make(map[string]bool)
	for _, mod := range m.Modalities.Input {
		inputMods[mod] = true
	}
	outputMods := make(map[string]bool)
	for _, mod := range m.Modalities.Output {
		outputMods[mod] = true
	}

	return model.ModelCapabilities{
		ToolCall:         m.ToolCall,
		StructuredOutput: m.StructuredOutput,
		Reasoning:        m.Reasoning,
		Attachment:       m.Attachment,
		InputModalities:  inputMods,
		OutputModalities: outputMods,
		ContextLimit:     m.Limit.Context,
		OutputLimit:      m.Limit.Output,
	}
}
