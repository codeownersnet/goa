package model

import (
	"fmt"
	"time"
)

type ProviderError struct {
	Provider   string
	StatusCode int
	Code       string
	Retryable  bool
	RetryAfter time.Duration
	RequestID  string
	Err        error
}

func (e *ProviderError) Error() string {
	message := e.Provider
	if e.StatusCode > 0 {
		message += fmt.Sprintf(" status %d", e.StatusCode)
	}
	if e.Code != "" {
		message += " (" + e.Code + ")"
	}
	if e.Retryable {
		message += " retryable"
	}
	if e.RetryAfter > 0 {
		message += fmt.Sprintf(" retry-after=%s", e.RetryAfter)
	}
	if e.RequestID != "" {
		message += " request-id=" + e.RequestID
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}
