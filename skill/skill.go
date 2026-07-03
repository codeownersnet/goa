package skill

type Skill struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	AllowedTools  []string          `json:"allowed_tools,omitempty"`
	Body          string            `json:"body,omitempty"`
	Location      string            `json:"location"`
}

func (s *Skill) Clone() *Skill {
	clone := &Skill{
		Name:          s.Name,
		Description:   s.Description,
		License:       s.License,
		Compatibility: s.Compatibility,
		Location:      s.Location,
		Body:          s.Body,
	}

	if s.Metadata != nil {
		clone.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			clone.Metadata[k] = v
		}
	}

	if s.AllowedTools != nil {
		clone.AllowedTools = make([]string, len(s.AllowedTools))
		copy(clone.AllowedTools, s.AllowedTools)
	}

	return clone
}
