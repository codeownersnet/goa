package model

import "fmt"

type StreamErrorKind string

const (
	StreamErrorIncomplete      StreamErrorKind = "incomplete_stream"
	StreamErrorMalformed       StreamErrorKind = "malformed_stream"
	StreamErrorLimit           StreamErrorKind = "stream_limit"
	StreamErrorInvalidToolCall StreamErrorKind = "invalid_tool_call"
)

type StreamError struct {
	Kind        StreamErrorKind
	Interrupted bool
	Err         error
}

func (e *StreamError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Kind, e.Err)
	}
	return string(e.Kind)
}

func (e *StreamError) Unwrap() error {
	return e.Err
}
