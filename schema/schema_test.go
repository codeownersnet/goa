package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	s := String("a name")
	assert.Equal(t, "string", s.Type)
	assert.Equal(t, "a name", s.Description)
}

func TestInt(t *testing.T) {
	s := Int("count")
	assert.Equal(t, "integer", s.Type)
	assert.Equal(t, "count", s.Description)
}

func TestFloat(t *testing.T) {
	s := Float("price")
	assert.Equal(t, "number", s.Type)
}

func TestBool(t *testing.T) {
	s := Bool("enabled")
	assert.Equal(t, "boolean", s.Type)
}

func TestArray(t *testing.T) {
	s := Array(String("item"), "list of items")
	assert.Equal(t, "array", s.Type)
	assert.NotNil(t, s.Items)
	assert.Equal(t, "string", s.Items.Type)
}

func TestObject(t *testing.T) {
	s := Object(
		map[string]*Schema{
			"name": String("person name"),
			"age":  Int("person age"),
		},
		"name",
	)
	assert.Equal(t, "object", s.Type)
	assert.Len(t, s.Properties, 2)
	assert.Equal(t, []string{"name"}, s.Required)
}
