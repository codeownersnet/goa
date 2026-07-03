package schema

type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties any                `json:"additionalProperties,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	Default              any                `json:"default,omitempty"`
	Nullable             bool               `json:"nullable,omitempty"`
	Format               string             `json:"format,omitempty"`
	MinItems             *int               `json:"minItems,omitempty"`
	MaxItems             *int               `json:"maxItems,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
	Defs                 map[string]*Schema `json:"$defs,omitempty"`
	Title                string             `json:"title,omitempty"`
}

func Object(properties map[string]*Schema, required ...string) *Schema {
	return &Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func String(desc string) *Schema {
	return &Schema{Type: "string", Description: desc}
}

func Int(desc string) *Schema {
	return &Schema{Type: "integer", Description: desc}
}

func Float(desc string) *Schema {
	return &Schema{Type: "number", Description: desc}
}

func Bool(desc string) *Schema {
	return &Schema{Type: "boolean", Description: desc}
}

func Array(items *Schema, desc string) *Schema {
	return &Schema{Type: "array", Items: items, Description: desc}
}
