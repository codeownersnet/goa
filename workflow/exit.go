package workflow

import (
	"fmt"
	"time"
)

type ExitCondition func(state map[string]any, elapsed time.Duration) bool

func newExitCondition(raw *exitWhenYAML) (ExitCondition, error) {
	if raw == nil {
		return nil, nil
	}
	var timeout time.Duration
	if raw.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(raw.Timeout)
		if err != nil {
			return nil, fmt.Errorf("exit_when.timeout: %w", err)
		}
	}
	expected := raw.State
	return func(state map[string]any, elapsed time.Duration) bool {
		if timeout > 0 && elapsed >= timeout {
			return true
		}
		if len(expected) > 0 {
			for k, v := range expected {
				actual, ok := state[k]
				if !ok || fmt.Sprintf("%v", actual) != v {
					return false
				}
			}
			return true
		}
		return false
	}, nil
}
