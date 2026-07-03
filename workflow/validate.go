package workflow

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type WorkflowError struct {
	Field   string
	Message string
	Err     error
}

func (e *WorkflowError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Field, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e *WorkflowError) Unwrap() error {
	return e.Err
}

const (
	wfTypeSequential = "sequential"
	wfTypeLoop       = "loop"

	stepTypeLLM        = "llm"
	stepTypeSequential = "sequential"
	stepTypeLoop       = "loop"
)

func validate(raw *workflowYAML) error {
	var errs []error

	if raw.Name == "" {
		errs = append(errs, &WorkflowError{Field: "name", Message: "required"})
	}
	if raw.Description == "" {
		errs = append(errs, &WorkflowError{Field: "description", Message: "required"})
	}
	if raw.Type == "" {
		raw.Type = wfTypeSequential
	}
	if raw.Type != wfTypeSequential && raw.Type != wfTypeLoop {
		errs = append(errs, &WorkflowError{Field: "type", Message: fmt.Sprintf("must be %q or %q", wfTypeSequential, wfTypeLoop)})
	}
	if len(raw.Steps) == 0 {
		errs = append(errs, &WorkflowError{Field: "steps", Message: "at least one step is required"})
	}

	if err := validateExitWhen(raw.ExitWhen, "exit_when"); err != nil {
		errs = append(errs, err)
	}

	if mcpErrs := validateMCPServers(raw.MCPServers, "mcp_servers"); len(mcpErrs) > 0 {
		errs = append(errs, mcpErrs...)
	}

	if provErrs := validateProviders(raw.Providers, "providers"); len(provErrs) > 0 {
		errs = append(errs, provErrs...)
	}

	if apErrs := validateAllowedPaths(raw.AllowedPaths, "allowed_paths"); len(apErrs) > 0 {
		errs = append(errs, apErrs...)
	}

	stepErrs := validateSteps(raw.Steps, "steps")
	errs = append(errs, stepErrs...)

	if len(errs) > 0 {
		return joinErrors(errs)
	}
	return nil
}

func validateSteps(steps []stepYAML, basePath string) []error {
	var errs []error
	seen := make(map[string][]int)

	for i, s := range steps {
		path := fmt.Sprintf("%s[%d]", basePath, i)

		if s.Name == "" {
			errs = append(errs, &WorkflowError{Field: path + ".name", Message: "required"})
		} else {
			seen[s.Name] = append(seen[s.Name], i)
		}

		if s.Type == "" {
			s.Type = stepTypeLLM
			steps[i].Type = stepTypeLLM
		}

		switch s.Type {
		case stepTypeLLM:
			if s.Model == "" {
				errs = append(errs, &WorkflowError{Field: path + ".model", Message: "required for llm steps"})
			}
			if s.Instruction == "" {
				errs = append(errs, &WorkflowError{Field: path + ".instruction", Message: "required for llm steps"})
			}
		case stepTypeLoop, stepTypeSequential:
			if len(s.Steps) == 0 {
				errs = append(errs, &WorkflowError{Field: path + ".steps", Message: "required for " + s.Type + " steps"})
			}
			subErrs := validateSteps(s.Steps, path+".steps")
			errs = append(errs, subErrs...)
		default:
			errs = append(errs, &WorkflowError{Field: path + ".type", Message: fmt.Sprintf("must be %q, %q, or %q", stepTypeLLM, stepTypeLoop, stepTypeSequential)})
		}

		if err := validateExitWhen(s.ExitWhen, path+".exit_when"); err != nil {
			errs = append(errs, err)
		}

		if s.GenerateConfig != nil {
			if err := validateGenerateConfig(s.GenerateConfig, path+".generate_config"); err != nil {
				errs = append(errs, err)
			}
		}
	}

	for name, indices := range seen {
		if len(indices) > 1 {
			idxStrs := make([]string, len(indices))
			for j, idx := range indices {
				idxStrs[j] = fmt.Sprintf("%d", idx)
			}
			errs = append(errs, &WorkflowError{
				Field:   basePath,
				Message: fmt.Sprintf("duplicate step name %q at indices %s", name, strings.Join(idxStrs, ",")),
			})
		}
	}

	return errs
}

func validateExitWhen(raw *exitWhenYAML, path string) error {
	if raw == nil {
		return nil
	}
	if raw.Timeout != "" {
		if _, err := time.ParseDuration(raw.Timeout); err != nil {
			return &WorkflowError{Field: path + ".timeout", Message: fmt.Sprintf("invalid duration %q", raw.Timeout), Err: err}
		}
	}
	return nil
}

func validateGenerateConfig(raw *generateConfigYAML, path string) error {
	if raw.ToolChoice != "" {
		valid := map[string]bool{"auto": true, "none": true, "required": true}
		if !valid[raw.ToolChoice] {
			return &WorkflowError{Field: path + ".tool_choice", Message: fmt.Sprintf("must be auto, none, or required, got %q", raw.ToolChoice)}
		}
	}
	return nil
}

func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("%d validation errors: %w", len(errs), multiError(errs))
}

type multiError []error

func (m multiError) Error() string {
	msgs := make([]string, len(m))
	for i, e := range m {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

func validateAllowedPaths(paths []string, basePath string) []error {
	if len(paths) == 0 {
		return nil
	}
	var errs []error
	for i, p := range paths {
		path := fmt.Sprintf("%s[%d]", basePath, i)
		if p == "" {
			errs = append(errs, &WorkflowError{Field: path, Message: "path must not be empty"})
		}
	}
	return errs
}

func validateProviders(providers map[string]providerYAML, basePath string) []error {
	if len(providers) == 0 {
		return nil
	}
	var errs []error
	for name, cfg := range providers {
		p := basePath + "." + name
		if !mcpServerNameRe.MatchString(name) {
			errs = append(errs, &WorkflowError{Field: p, Message: "name must match ^[a-zA-Z][a-zA-Z0-9_-]*$"})
		}
		if cfg.APIBase == "" {
			errs = append(errs, &WorkflowError{Field: p + ".api_base", Message: "required"})
		}
		if cfg.Type != "" && cfg.Type != "openai_compat" && cfg.Type != "anthropic" {
			errs = append(errs, &WorkflowError{Field: p + ".type", Message: fmt.Sprintf("must be %q or %q", "openai_compat", "anthropic")})
		}
	}
	return errs
}

var mcpServerNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

func validateMCPServers(servers map[string]mcpServerYAML, basePath string) []error {
	if len(servers) == 0 {
		return nil
	}
	var errs []error
	for name, cfg := range servers {
		p := basePath + "." + name
		if !mcpServerNameRe.MatchString(name) {
			errs = append(errs, &WorkflowError{Field: p, Message: "name must match ^[a-zA-Z][a-zA-Z0-9_-]*$"})
		}
		if cfg.Command == "" && cfg.URL == "" {
			errs = append(errs, &WorkflowError{Field: p, Message: "must specify command or url"})
		}
		if cfg.Command != "" && cfg.URL != "" {
			errs = append(errs, &WorkflowError{Field: p, Message: "must specify command or url, not both"})
		}
		if cfg.ConnectTimeout != "" {
			if _, err := time.ParseDuration(cfg.ConnectTimeout); err != nil {
				errs = append(errs, &WorkflowError{Field: p + ".connect_timeout", Message: fmt.Sprintf("invalid duration %q", cfg.ConnectTimeout), Err: err})
			}
		}
		if cfg.ToolTimeout != "" {
			if _, err := time.ParseDuration(cfg.ToolTimeout); err != nil {
				errs = append(errs, &WorkflowError{Field: p + ".tool_timeout", Message: fmt.Sprintf("invalid duration %q", cfg.ToolTimeout), Err: err})
			}
		}
		for k := range cfg.Headers {
			if k == "" {
				errs = append(errs, &WorkflowError{Field: p + ".headers", Message: "header key must not be empty"})
			}
		}
	}
	return errs
}
