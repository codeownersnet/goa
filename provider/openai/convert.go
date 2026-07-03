package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/model"
	internal "github.com/codeownersnet/goa/provider/internal"
)

func toOpenAIMessages(contents []*content.Content, systemInstruction string) []openaiMessage {
	msgs := make([]openaiMessage, 0, len(contents)+1)

	if systemInstruction != "" {
		msgs = append(msgs, openaiMessage{Role: "system", Content: systemInstruction})
	}

	for _, c := range contents {
		switch c.Role {
		case content.RoleSystem:
			msgs = append(msgs, openaiMessage{
				Role:    "system",
				Content: partsToOpenAIContent(c.Parts),
			})
		case content.RoleUser:
			msgs = append(msgs, openaiMessage{
				Role:    "user",
				Content: partsToOpenAIContent(c.Parts),
			})
		case content.RoleModel:
			msg := openaiMessage{Role: "assistant"}
			for _, p := range c.Parts {
				if p.Text != nil {
					msg.Content = p.Text.Text
				}
				if p.FunctionCall != nil {
					msg.ToolCalls = append(msg.ToolCalls, openaiToolCall{
						ID:   p.FunctionCall.ID,
						Type: "function",
						Function: openaiFunction{
							Name:      p.FunctionCall.Name,
							Arguments: internal.MustMarshal(p.FunctionCall.Args),
						},
					})
				}
				if p.Thinking != nil {
					msg.ReasoningContent = p.Thinking.Text
				}
			}
			msgs = append(msgs, msg)
		case content.RoleTool:
			for _, p := range c.Parts {
				if p.FunctionResponse != nil {
					respContent := internal.MustMarshal(p.FunctionResponse.Response)
					if p.FunctionResponse.IsError {
						respContent = `{"error": ` + respContent + `}`
					}
					msgs = append(msgs, openaiMessage{
						Role:       "tool",
						ToolCallID: p.FunctionResponse.ID,
						Content:    respContent,
					})
				}
			}
		}
	}
	return msgs
}

func partsToOpenAIContent(parts []content.Part) any {
	if len(parts) == 1 && parts[0].Text != nil {
		return parts[0].Text.Text
	}

	items := make([]any, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.Text != nil:
			items = append(items, map[string]any{
				"type": "text",
				"text": p.Text.Text,
			})
		case p.InlineData != nil:
			items = append(items, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": fmt.Sprintf("data:%s;base64,%s", p.InlineData.MIMEType, p.InlineData.Data),
				},
			})
		}
	}
	if len(items) == 0 {
		return ""
	}
	return items
}

func fromOpenAIResponse(resp *chatCompletionResponse) *model.ModelResponse {
	if len(resp.Choices) == 0 {
		return &model.ModelResponse{ErrorCode: "empty_response"}
	}

	choice := resp.Choices[0]
	parts := openaiMessageToParts(&choice.Message)

	return &model.ModelResponse{
		Content:      content.NewContent(content.RoleModel, parts...),
		Usage:        fromOpenAIUsage(resp.Usage),
		FinishReason: fromOpenAIFinishReason(choice.FinishReason),
		ModelVersion: resp.Model,
	}
}

func openaiMessageToParts(msg *openaiMessage) []content.Part {
	parts := make([]content.Part, 0, len(msg.ToolCalls)+2)
	if text, ok := msg.Content.(string); ok && text != "" {
		parts = append(parts, content.NewTextPart(text))
	}
	if msg.ReasoningContent != "" {
		parts = append(parts, content.NewThinkingPart(msg.ReasoningContent))
	}
	for _, tc := range msg.ToolCalls {
		parts = append(parts, content.Part{FunctionCall: &content.FunctionCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: parseArgsJSON(tc.Function.Arguments),
		}})
	}
	return parts
}

func fromOpenAIUsage(u *openaiUsage) *model.Usage {
	if u == nil {
		return nil
	}
	return &model.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		CacheReadTokens:  u.PromptTokensDetails.CachedTokens,
	}
}

func fromOpenAIFinishReason(reason string) model.FinishReason {
	switch reason {
	case "stop":
		return model.FinishReasonStop
	case "tool_calls":
		return model.FinishReasonToolCall
	case "length":
		return model.FinishReasonMaxTokens
	case "content_filter":
		return model.FinishReasonSafety
	default:
		return model.FinishReasonUnknown
	}
}

func toolsToOpenAITools(declarations []content.ToolDeclaration) []openaiTool {
	tools := make([]openaiTool, 0, len(declarations))
	for _, d := range declarations {
		tools = append(tools, openaiTool{
			Type: "function",
			Function: openaiFunctionDef{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  internal.SchemaToOpenAI(d.Parameters),
			},
		})
	}
	return tools
}

func (a *Adapter) buildRequestBody(req *model.ModelRequest) (*chatCompletionRequest, error) {
	cfg := req.Config
	if cfg == nil {
		cfg = &model.GenerateConfig{}
	}

	body := &chatCompletionRequest{
		Model:    a.cfg.ProviderModelID,
		Messages: toOpenAIMessages(req.Contents, cfg.SystemInstruction),
		Stream:   false,
	}

	if cfg.Temperature != nil {
		body.Temperature = cfg.Temperature
	}
	if cfg.TopP != nil {
		body.TopP = cfg.TopP
	}
	if cfg.MaxTokens > 0 {
		body.MaxTokens = cfg.MaxTokens
	}
	if len(cfg.StopSequences) > 0 {
		body.Stop = cfg.StopSequences[0]
	}

	switch cfg.ToolChoice.Mode {
	case model.ToolChoiceAuto:
		body.ToolChoice = "auto"
	case model.ToolChoiceNone:
		body.ToolChoice = "none"
	case model.ToolChoiceRequired:
		body.ToolChoice = "required"
	case model.ToolChoiceSpecific:
		body.ToolChoice = map[string]any{
			"type":     "function",
			"function": map[string]any{"name": cfg.ToolChoice.Name},
		}
	}

	toolDecls := make([]content.ToolDeclaration, 0, len(req.Tools))
	for _, t := range req.Tools {
		if td, ok := t.(content.ToolDeclaration); ok {
			toolDecls = append(toolDecls, td)
		}
	}
	if len(toolDecls) > 0 {
		body.Tools = toolsToOpenAITools(toolDecls)
	}

	if cfg.ResponseSchema != nil {
		body.ResponseFormat = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "response",
				"schema": internal.SchemaToOpenAI(cfg.ResponseSchema),
			},
		}
	}

	return body, nil
}

func (a *Adapter) newHTTPRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	return internal.NewHTTPRequest(ctx, internal.AuthConfig{
		APIKey:   a.cfg.APIKey,
		AuthType: "bearer",
	}, a.cfg.BaseURL, method, path, body)
}

func parseArgsJSON(s string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(s), &args); err != nil {
		return map[string]any{"raw": s}
	}
	return args
}

type chatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []openaiMessage `json:"messages"`
	Stream         bool            `json:"stream"`
	Temperature    *float64        `json:"temperature,omitempty"`
	TopP           *float64        `json:"top_p,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Stop           any             `json:"stop,omitempty"`
	Tools          []openaiTool    `json:"tools,omitempty"`
	ToolChoice     any             `json:"tool_choice,omitempty"`
	ResponseFormat any             `json:"response_format,omitempty"`
}

type openaiMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
}

type openaiToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Index    int            `json:"index,omitempty"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type     string            `json:"type"`
	Function openaiFunctionDef `json:"function"`
}

type openaiFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *openaiUsage           `json:"usage,omitempty"`
}

type chatCompletionChoice struct {
	Index        int           `json:"index"`
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
	Delta        *openaiDelta  `json:"delta,omitempty"`
}

type openaiDelta struct {
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []toolCallChunk `json:"tool_calls,omitempty"`
}

type toolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type openaiUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens,omitempty"`
	} `json:"prompt_tokens_details,omitempty"`
}

type chatCompletionChunk struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
}
