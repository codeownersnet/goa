package session

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var eventCounter atomic.Int64

func NewEvent(invocationID string) *Event {
	id := eventCounter.Add(1)
	return &Event{
		ID:           fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), id),
		InvocationID: invocationID,
		Timestamp:    time.Now(),
		Actions:      EventActions{StateDelta: make(map[string]any), ArtifactDelta: make(map[string]int64)},
	}
}

func (e *Event) IsFinalResponse() bool {
	if e.Actions.SkipSummarization {
		return true
	}
	return !hasFunctionCalls(e) && !hasFunctionResponses(e) && !e.Partial
}

func hasFunctionCalls(e *Event) bool {
	if e.ModelResponse == nil {
		return false
	}
	for _, part := range e.ModelResponse.Parts {
		if part.FunctionCall != nil {
			return true
		}
	}
	return false
}

func hasFunctionResponses(e *Event) bool {
	if e.ModelResponse == nil {
		return false
	}
	for _, part := range e.ModelResponse.Parts {
		if part.FunctionResponse != nil {
			return true
		}
	}
	return false
}

func (e *Event) Text() string {
	if e.ModelResponse == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range e.ModelResponse.Parts {
		if part.Text != nil {
			b.WriteString(part.Text.Text)
		}
	}
	return b.String()
}
