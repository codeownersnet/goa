package model

type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"
	FinishReasonToolCall  FinishReason = "tool_call"
	FinishReasonMaxTokens FinishReason = "max_tokens"
	FinishReasonSafety    FinishReason = "safety"
	FinishReasonRecursion FinishReason = "recursion"
	FinishReasonCancelled FinishReason = "cancelled"
	FinishReasonUnknown   FinishReason = "unknown"
)

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}
