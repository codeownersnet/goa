package content

import "github.com/codeownersnet/goa/schema"

type ToolDeclaration struct {
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Parameters     *schema.Schema `json:"parameters,omitempty"`
	ResponseSchema *schema.Schema `json:"response_schema,omitempty"`
	IsLongRunning  bool           `json:"is_long_running,omitempty"`
}
