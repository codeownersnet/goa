package internal

import (
	"github.com/codeownersnet/goa/schema"
)

func SchemaToOpenAI(s *schema.Schema) map[string]any {
	if s == nil {
		return nil
	}
	m := map[string]any{
		"type": s.Type,
	}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for k, v := range s.Properties {
			props[k] = SchemaToOpenAI(v)
		}
		m["properties"] = props
	}
	if len(s.Required) > 0 {
		m["required"] = s.Required
	}
	if s.Items != nil {
		m["items"] = SchemaToOpenAI(s.Items)
	}
	if s.AdditionalProperties != nil {
		m["additionalProperties"] = s.AdditionalProperties
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
	return m
}
