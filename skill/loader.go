package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ParseError struct {
	Field   string
	Message string
	Err     error
}

func (e *ParseError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("parse error in field %q: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

func (e *ParseError) Unwrap() error { return e.Err }

type ValidationError struct {
	Field   string
	Message string
	Err     error
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error in field %q: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

func (e *ValidationError) Unwrap() error { return e.Err }

type skillFrontMatter struct {
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	License       string         `yaml:"license"`
	Compatibility string         `yaml:"compatibility"`
	AllowedTools  string         `yaml:"allowed-tools"`
	Metadata      map[string]any `yaml:"metadata"`
}

func Load(dir string) (*Skill, error) {
	var data []byte
	var path string

	for _, name := range []string{"SKILL.md", "skill.md"} {
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		if err == nil {
			data = b
			path = p
			break
		}
	}

	if data == nil {
		return nil, &ParseError{Message: fmt.Sprintf("SKILL.md not found in %s", dir)}
	}

	s, err := parse(data)
	if err != nil {
		return nil, err
	}

	s.Location = filepath.Dir(path)
	return s, nil
}

func parse(data []byte) (*Skill, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return nil, &ParseError{Message: "missing YAML frontmatter delimiter"}
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) != 2 {
		return nil, &ParseError{Message: "missing closing YAML frontmatter delimiter"}
	}

	front := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])

	var raw skillFrontMatter
	if err := yaml.Unmarshal([]byte(front), &raw); err != nil {
		raw = parseFrontMatterLenient(front)
	}

	s := &Skill{
		Name:          raw.Name,
		Description:   raw.Description,
		License:       raw.License,
		Compatibility: raw.Compatibility,
		Body:          body,
	}

	if raw.AllowedTools != "" {
		tools := strings.Fields(raw.AllowedTools)
		for i := range tools {
			tools[i] = strings.TrimSpace(tools[i])
		}
		if len(tools) > 0 {
			s.AllowedTools = tools
		}
	}

	if raw.Metadata != nil {
		s.Metadata = make(map[string]string, len(raw.Metadata))
		for k, v := range raw.Metadata {
			switch val := v.(type) {
			case string:
				s.Metadata[k] = val
			case int:
				s.Metadata[k] = fmt.Sprintf("%d", val)
			case bool:
				s.Metadata[k] = fmt.Sprintf("%t", val)
			case float64:
				s.Metadata[k] = fmt.Sprintf("%v", val)
			default:
				s.Metadata[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	if err := Validate(s); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, err
		}
		return nil, &ValidationError{Message: err.Error(), Err: err}
	}

	return s, nil
}

func parseFrontMatterLenient(front string) skillFrontMatter {
	var raw skillFrontMatter
	for _, line := range strings.Split(front, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			raw.Name = value
		case "description":
			raw.Description = value
		case "license":
			raw.License = value
		case "compatibility":
			raw.Compatibility = value
		case "allowed-tools":
			raw.AllowedTools = value
		}
	}
	return raw
}
