package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type workflowYAML struct {
	Name          string                   `yaml:"name"`
	Description   string                   `yaml:"description"`
	Type          string                   `yaml:"type"`
	MaxIterations int                      `yaml:"max_iterations"`
	ExitWhen      *exitWhenYAML            `yaml:"exit_when"`
	Providers     map[string]providerYAML  `yaml:"providers"`
	AllowedPaths  []string                 `yaml:"allowed_paths"`
	MCPServers    map[string]mcpServerYAML `yaml:"mcp_servers"`
	Steps         []stepYAML               `yaml:"steps"`
}

type providerYAML struct {
	APIBase string   `yaml:"api_base"`
	Env     []string `yaml:"env"`
	Type    string   `yaml:"type"`
}

type mcpServerYAML struct {
	Command        string            `yaml:"command"`
	URL            string            `yaml:"url"`
	Headers        map[string]string `yaml:"headers"`
	Env            []string          `yaml:"env"`
	ConnectTimeout string            `yaml:"connect_timeout"`
	ToolTimeout    string            `yaml:"tool_timeout"`
}

type stepYAML struct {
	Name           string              `yaml:"name"`
	Type           string              `yaml:"type"`
	Model          string              `yaml:"model"`
	Instruction    string              `yaml:"instruction"`
	Tools          []string            `yaml:"tools"`
	Skills         []string            `yaml:"skills"`
	AutoSkills     *bool               `yaml:"auto_skills"`
	NoQuestions    *bool               `yaml:"no_questions"`
	MaxIterations  int                 `yaml:"max_iterations"`
	Steps          []stepYAML          `yaml:"steps"`
	ExitWhen       *exitWhenYAML       `yaml:"exit_when"`
	GenerateConfig *generateConfigYAML `yaml:"generate_config"`
}

type exitWhenYAML struct {
	State   map[string]string `yaml:"state"`
	Timeout string            `yaml:"timeout"`
}

type generateConfigYAML struct {
	Temperature   *float64 `yaml:"temperature"`
	TopP          *float64 `yaml:"top_p"`
	MaxTokens     int      `yaml:"max_tokens"`
	StopSequences []string `yaml:"stop_sequences"`
	ToolChoice    string   `yaml:"tool_choice"`
}

func parse(data []byte) (*workflowYAML, error) {
	var raw workflowYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return &raw, nil
}

func parseFile(path string) (*workflowYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow file: %w", err)
	}
	return parse(data)
}
