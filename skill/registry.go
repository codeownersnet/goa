package skill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func defaultSkillDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		".agents/skills",
		filepath.Join(home, ".agents/skills"),
		".goa/skills",
		filepath.Join(home, ".goa/skills"),
	}
}

type RegistryOption func(*Registry)

type Registry struct {
	skillDirs  []string
	skills     map[string]*Skill
	runScripts bool
}

func WithSkillDirs(dirs ...string) RegistryOption {
	return func(r *Registry) {
		r.skillDirs = dirs
	}
}

func WithRunScripts(enabled bool) RegistryOption {
	return func(r *Registry) {
		r.runScripts = enabled
	}
}

func NewRegistry(opts ...RegistryOption) *Registry {
	dirs := defaultSkillDirs()
	r := &Registry{
		skillDirs: make([]string, 0, len(dirs)),
		skills:    make(map[string]*Skill),
	}
	for _, d := range dirs {
		r.skillDirs = append(r.skillDirs, d)
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Registry) Discover() error {
	for _, dir := range r.skillDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read skill dir %q: %w", dir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillDir := filepath.Join(dir, entry.Name())
			s, err := Load(skillDir)
			if err != nil {
				var pe *ParseError
				if errors.As(err, &pe) {
					continue
				}
				var ve *ValidationError
				if errors.As(err, &ve) {
					continue
				}
				continue
			}

			skillMeta := &Skill{
				Name:        s.Name,
				Description: s.Description,
				Location:    s.Location,
			}
			r.skills[s.Name] = skillMeta
		}
	}
	return nil
}

func (r *Registry) Lookup(name string) (*Skill, bool) {
	s, ok := r.skills[name]
	if !ok {
		return nil, false
	}
	return s.Clone(), true
}

func (r *Registry) List() []*Skill {
	list := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		list = append(list, s.Clone())
	}
	return list
}

func (r *Registry) Activate(name string) (*Skill, error) {
	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}

	full, err := Load(s.Location)
	if err != nil {
		return nil, fmt.Errorf("activate skill %q: %w", name, err)
	}

	r.skills[name] = full
	return full.Clone(), nil
}

func (r *Registry) ReadResource(name string, relativePath string) ([]byte, error) {
	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}

	cleanPath := filepath.Clean(relativePath)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid relative path: %q", relativePath)
	}

	for _, prefix := range []string{"references", "assets"} {
		fullPath := filepath.Join(s.Location, prefix, cleanPath)
		data, err := os.ReadFile(fullPath)
		if err == nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("resource %q not found for skill %q", relativePath, name)
}

func (r *Registry) RunScript(ctx context.Context, name string, scriptPath string) ([]byte, error) {
	if !r.runScripts {
		return nil, fmt.Errorf("script execution is disabled")
	}

	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}

	cleanPath := filepath.Clean(scriptPath)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid script path: %q", scriptPath)
	}

	fullPath := filepath.Join(s.Location, "scripts", cleanPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("script %q not found: %w", scriptPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("script path %q is a directory", scriptPath)
	}

	if info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("script %q is not executable", scriptPath)
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, fullPath)
	cmd.Dir = s.Location
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run script %q: %w", scriptPath, err)
	}
	return output, nil
}
