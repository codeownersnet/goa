package mcptoolset

import (
	"encoding/json"

	"github.com/codeownersnet/goa/schema"
)

func convertInputSchema(inputSchema any) *schema.Schema {
	if inputSchema == nil {
		return schema.Object(nil)
	}

	switch s := inputSchema.(type) {
	case map[string]any:
		return convertSchemaMap(s)
	case *schema.Schema:
		return s
	default:
		data, err := json.Marshal(inputSchema)
		if err != nil {
			return schema.Object(nil)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return schema.Object(nil)
		}
		return convertSchemaMap(m)
	}
}

func convertSchemaMap(m map[string]any) *schema.Schema {
	s := &schema.Schema{}

	if t, ok := m["type"].(string); ok {
		s.Type = t
	}
	if desc, ok := m["description"].(string); ok {
		s.Description = desc
	}
	if format, ok := m["format"].(string); ok {
		s.Format = format
	}
	if title, ok := m["title"].(string); ok {
		s.Title = title
	}
	if nullable, ok := m["nullable"].(bool); ok {
		s.Nullable = nullable
	}

	if props, ok := m["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*schema.Schema, len(props))
		for k, v := range props {
			if sub, ok := v.(map[string]any); ok {
				s.Properties[k] = convertSchemaMap(sub)
			}
		}
	}

	if req, ok := m["required"].([]any); ok {
		s.Required = make([]string, 0, len(req))
		for _, r := range req {
			if rs, ok := r.(string); ok {
				s.Required = append(s.Required, rs)
			}
		}
	}

	if items, ok := m["items"].(map[string]any); ok {
		s.Items = convertSchemaMap(items)
	}

	if ap, exists := m["additionalProperties"]; exists {
		s.AdditionalProperties = ap
	}

	if enum, ok := m["enum"].([]any); ok {
		s.Enum = enum
	}

	if def, ok := m["default"]; ok {
		s.Default = def
	}

	if minLen, ok := toInt(m["minLength"]); ok {
		s.MinLength = &minLen
	}
	if maxLen, ok := toInt(m["maxLength"]); ok {
		s.MaxLength = &maxLen
	}
	if minItems, ok := toInt(m["minItems"]); ok {
		s.MinItems = &minItems
	}
	if maxItems, ok := toInt(m["maxItems"]); ok {
		s.MaxItems = &maxItems
	}
	if minimum, ok := toFloat64(m["minimum"]); ok {
		s.Minimum = &minimum
	}
	if maximum, ok := toFloat64(m["maximum"]); ok {
		s.Maximum = &maximum
	}

	if s.Type == "" {
		s.Type = "object"
	}

	return s
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	}
	return 0, false
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}
