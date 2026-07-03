package model

import (
	"context"
	"iter"

	"github.com/codeownersnet/goa/content"
)

type Model interface {
	Name() string
	GenerateContent(ctx context.Context, req *ModelRequest, stream bool) iter.Seq2[*ModelResponse, error]
	Capabilities() ModelCapabilities
}

type ModelRequest struct {
	Model    string             `json:"model"`
	Contents []*content.Content `json:"contents"`
	Config   *GenerateConfig    `json:"config"`
	Tools    map[string]any     `json:"-"`
}

type ModelResponse struct {
	Content          *content.Content `json:"content,omitempty"`
	Usage            *Usage           `json:"usage,omitempty"`
	FinishReason     FinishReason     `json:"finish_reason,omitempty"`
	ModelVersion     string           `json:"model_version,omitempty"`
	Partial          bool             `json:"partial,omitempty"`
	TurnComplete     bool             `json:"turn_complete,omitempty"`
	ProviderMetadata map[string]any   `json:"provider_metadata,omitempty"`
	ErrorCode        string           `json:"error_code,omitempty"`
	ErrorMessage     string           `json:"error_message,omitempty"`
	Interrupted      bool             `json:"interrupted,omitempty"`
}
