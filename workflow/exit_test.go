package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewExitConditionNil(t *testing.T) {
	cond, err := newExitCondition(nil)
	assert.NoError(t, err)
	assert.Nil(t, cond)
}

func TestExitConditionStateMatch(t *testing.T) {
	cond, err := newExitCondition(&exitWhenYAML{
		State: map[string]string{"status": "complete"},
	})
	assert.NoError(t, err)

	assert.False(t, cond(map[string]any{}, 0))
	assert.False(t, cond(map[string]any{"status": "incomplete"}, 0))
	assert.True(t, cond(map[string]any{"status": "complete"}, 0))
}

func TestExitConditionStateMultipleKeys(t *testing.T) {
	cond, err := newExitCondition(&exitWhenYAML{
		State: map[string]string{"a": "1", "b": "2"},
	})
	assert.NoError(t, err)

	assert.False(t, cond(map[string]any{"a": "1"}, 0))
	assert.True(t, cond(map[string]any{"a": "1", "b": "2"}, 0))
}

func TestExitConditionTimeout(t *testing.T) {
	cond, err := newExitCondition(&exitWhenYAML{
		Timeout: "1s",
	})
	assert.NoError(t, err)

	assert.False(t, cond(map[string]any{}, 500*time.Millisecond))
	assert.True(t, cond(map[string]any{}, 2*time.Second))
}

func TestExitConditionStateOrTimeout(t *testing.T) {
	cond, err := newExitCondition(&exitWhenYAML{
		State:   map[string]string{"done": "true"},
		Timeout: "5m",
	})
	assert.NoError(t, err)

	assert.False(t, cond(map[string]any{}, 0))
	assert.True(t, cond(map[string]any{"done": "true"}, 0))
	assert.True(t, cond(map[string]any{}, 10*time.Minute))
}

func TestExitConditionEmptyConfig(t *testing.T) {
	cond, err := newExitCondition(&exitWhenYAML{})
	assert.NoError(t, err)
	assert.False(t, cond(map[string]any{}, 0))
}

func TestExitConditionInvalidTimeout(t *testing.T) {
	_, err := newExitCondition(&exitWhenYAML{
		Timeout: "not-a-duration",
	})
	assert.Error(t, err)
}

func TestExitConditionStateTypeCoercion(t *testing.T) {
	cond, err := newExitCondition(&exitWhenYAML{
		State: map[string]string{"count": "42"},
	})
	assert.NoError(t, err)

	assert.True(t, cond(map[string]any{"count": 42}, 0))
	assert.True(t, cond(map[string]any{"count": "42"}, 0))
}
