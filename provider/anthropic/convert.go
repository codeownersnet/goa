package anthropic

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	internal "github.com/codeownersnet/goa/provider/internal"
	"github.com/codeownersnet/goa/schema"
)

func toAnthropicMessages(contents []*content.Content) []anthropicMessage {
	msgs := make([]anthropicMessage, 0, len(contents))

	for _, c := range contents {
		switch c.Role {
		case content.RoleUser:
			blocks := make([]anthropicContentBlock, 0, len(c.Parts))
			for _, p := range c.Parts {
				switch {
				case p.Text != nil:
					blocks = append(blocks, anthropicContentBlock{
						Type: "text",
						Text: p.Text.Text,
					})
				case p.InlineData != nil:
					blocks = append(blocks, anthropicContentBlock{
						Type: "image",
						Source: &anthropicSource{
							Type:      "base64",
							MediaType: p.InlineData.MIMEType,
							Data:      string(p.InlineData.Data),
						},
					})
				case p.FunctionResponse != nil:
					resultContent := internal.MustMarshal(p.FunctionResponse.Response)
					block := anthropicContentBlock{
						Type:      "tool_result",
						ToolUseID: p.FunctionResponse.ID,
						Content:   resultContent,
					}
					if p.FunctionResponse.IsError {
						block.IsError = true
					}
					blocks = append(blocks, block)
				}
			}
			if len(blocks) > 0 {
				msgs = append(msgs, anthropicMessage{Role: "user", Content: blocks})
			}

		case content.RoleModel:
			blocks := make([]anthropicContentBlock, 0, len(c.Parts))
			for _, p := range c.Parts {
				switch {
				case p.Text != nil:
					blocks = append(blocks, anthropicContentBlock{
						Type: "text",
						Text: p.Text.Text,
					})
				case p.FunctionCall != nil:
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    p.FunctionCall.ID,
						Name:  p.FunctionCall.Name,
						Input: p.FunctionCall.Args,
					})
				case p.Thinking != nil:
					blocks = append(blocks, anthropicContentBlock{
						Type:     "thinking",
						Thinking: p.Thinking.Text,
					})
				}
			}
			if len(blocks) > 0 {
				msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
			}

		case content.RoleTool:
			blocks := make([]anthropicContentBlock, 0, len(c.Parts))
			for _, p := range c.Parts {
				if p.FunctionResponse != nil {
					resultContent := internal.MustMarshal(p.FunctionResponse.Response)
					block := anthropicContentBlock{
						Type:      "tool_result",
						ToolUseID: p.FunctionResponse.ID,
						Content:   resultContent,
					}
					if p.FunctionResponse.IsError {
						block.IsError = true
					}
					blocks = append(blocks, block)
				}
			}
			if len(blocks) > 0 {
				msgs = append(msgs, anthropicMessage{Role: "user", Content: blocks})
			}
		}
	}

	if len(msgs) > 0 && msgs[len(msgs)-1].Role == "assistant" {
		msgs = append(msgs, anthropicMessage{
			Role:    "user",
			Content: []anthropicContentBlock{{Type: "text", Text: "Continue"}},
		})
	}

	return msgs
}

func fromAnthropicResponse(resp *messagesResponse) *model.ModelResponse {
	parts := anthropicContentBlocksToParts(resp.Content)

	return &model.ModelResponse{
		Content:      content.NewContent(content.RoleModel, parts...),
		Usage:        fromAnthropicUsage(resp.Usage),
		FinishReason: fromAnthropicStopReason(resp.StopReason),
		ModelVersion: resp.Model,
	}
}

func anthropicContentBlocksToParts(blocks []anthropicContentBlock) []content.Part {
	parts := make([]content.Part, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, content.NewTextPart(b.Text))
		case "tool_use":
			parts = append(parts, content.Part{FunctionCall: &content.FunctionCall{
				ID:   b.ID,
				Name: b.Name,
				Args: toMapStringAny(b.Input),
			}})
		case "thinking":
			parts = append(parts, content.NewThinkingPart(b.Thinking))
		}
	}
	return parts
}

func fromAnthropicUsage(u *anthropicUsage) *model.Usage {
	if u == nil {
		return nil
	}
	return &model.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

func fromAnthropicStopReason(reason string) model.FinishReason {
	switch reason {
	case "end_turn":
		return model.FinishReasonStop
	case "tool_use":
		return model.FinishReasonToolCall
	case "max_tokens":
		return model.FinishReasonMaxTokens
	case "stop_sequence":
		return model.FinishReasonStop
	default:
		return model.FinishReasonUnknown
	}
}

func toolsToAnthropicTools(declarations []content.ToolDeclaration) []anthropicTool {
	tools := make([]anthropicTool, 0, len(declarations))
	for _, d := range declarations {
		tools = append(tools, anthropicTool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: schemaToAnthropicInputSchema(d.Parameters),
		})
	}
	return tools
}

func schemaToAnthropicInputSchema(s *schema.Schema) map[string]any {
	if s == nil {
		return map[string]any{"type": "object"}
	}
	return schemaToAnthropicMap(s, true)
}

func schemaToAnthropicMap(s *schema.Schema, isTopLevel bool) map[string]any {
	m := map[string]any{
		"type": s.Type,
	}
	if !isTopLevel && s.Description != "" {
		m["description"] = s.Description
	}
	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for k, v := range s.Properties {
			props[k] = schemaToAnthropicMap(v, false)
		}
		m["properties"] = props
	}
	if len(s.Required) > 0 {
		m["required"] = s.Required
	}
	if s.Items != nil {
		m["items"] = schemaToAnthropicMap(s.Items, false)
	}
	if len(s.Enum) > 0 {
		m["enum"] = s.Enum
	}
	if s.Nullable {
		m["nullable"] = true
	}
	if s.Format != "" {
		m["format"] = s.Format
	}
	if s.AdditionalProperties != nil {
		m["additionalProperties"] = s.AdditionalProperties
	}
	if s.MinItems != nil {
		m["minItems"] = *s.MinItems
	}
	if s.MaxItems != nil {
		m["maxItems"] = *s.MaxItems
	}
	if s.MinLength != nil {
		m["minLength"] = *s.MinLength
	}
	if s.MaxLength != nil {
		m["maxLength"] = *s.MaxLength
	}
	if s.Minimum != nil {
		m["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		m["maximum"] = *s.Maximum
	}
	return m
}

func (a *Adapter) buildRequestBody(req *model.ModelRequest) (*messagesRequest, error) {
	cfg := req.Config
	if cfg == nil {
		cfg = &model.GenerateConfig{}
	}

	msgs := toAnthropicMessages(req.Contents)

	body := &messagesRequest{
		Model:     a.cfg.ProviderModelID,
		Messages:  msgs,
		MaxTokens: 4096,
	}

	if cfg.SystemInstruction != "" {
		body.System = cfg.SystemInstruction
	}
	if cfg.MaxTokens > 0 {
		body.MaxTokens = cfg.MaxTokens
	}
	if cfg.Temperature != nil {
		body.Temperature = cfg.Temperature
	}
	if cfg.TopP != nil {
		body.TopP = cfg.TopP
	}

	if cfg.ThinkingConfig != nil && cfg.ThinkingConfig.BudgetTokens > 0 {
		body.Thinking = &anthropicThinking{
			Type:         "enabled",
			BudgetTokens: cfg.ThinkingConfig.BudgetTokens,
		}
	}

	toolDecls := make([]content.ToolDeclaration, 0, len(req.Tools))
	for _, t := range req.Tools {
		if td, ok := t.(content.ToolDeclaration); ok {
			toolDecls = append(toolDecls, td)
		}
	}
	if len(toolDecls) > 0 {
		body.Tools = toolsToAnthropicTools(toolDecls)
	}

	return body, nil
}

func (a *Adapter) newHTTPRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	return internal.NewHTTPRequest(ctx, internal.AuthConfig{
		APIKey:       a.cfg.APIKey,
		AuthType:     "x-api-key",
		ExtraHeaders: map[string]string{"anthropic-version": "2023-06-01"},
	}, a.cfg.BaseURL, method, path, body)
}

func toMapStringAny(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	data, _ := json.Marshal(v)
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

type messagesRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	System      any                `json:"system,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Thinking    *anthropicThinking `json:"thinking,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string           `json:"type"`
	Text      string           `json:"text,omitempty"`
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Input     any              `json:"input,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	Content   any              `json:"content,omitempty"`
	IsError   bool             `json:"is_error,omitempty"`
	Thinking  string           `json:"thinking,omitempty"`
	Source    *anthropicSource `json:"source,omitempty"`
}

type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type messagesResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      *anthropicUsage         `json:"usage,omitempty"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

type anthropicSSE struct {
	Type         string                 `json:"type"`
	Index        int                    `json:"index,omitempty"`
	Message      *messagesResponse      `json:"message,omitempty"`
	Delta        *anthropicDelta        `json:"delta,omitempty"`
	Usage        *anthropicSSEUsage     `json:"usage,omitempty"`
	ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type anthropicSSEUsage struct {
	OutputTokens int `json:"output_tokens,omitempty"`
}
