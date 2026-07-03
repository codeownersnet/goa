package skill

import (
	"fmt"
	"path/filepath"
	"unicode"
)

func ValidateName(name string) error {
	if len(name) == 0 {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if len(name) > 64 {
		return &ValidationError{Field: "name", Message: "name must be at most 64 characters"}
	}
	if name[0] == '-' {
		return &ValidationError{Field: "name", Message: "name must not start with a hyphen"}
	}
	if name[len(name)-1] == '-' {
		return &ValidationError{Field: "name", Message: "name must not end with a hyphen"}
	}

	for i, r := range name {
		if r == '-' {
			if i > 0 && name[i-1] == '-' {
				return &ValidationError{Field: "name", Message: "name must not contain consecutive hyphens"}
			}
			continue
		}
		if !unicode.IsLower(r) && !unicode.IsDigit(r) {
			return &ValidationError{Field: "name", Message: "name must contain only lowercase a-z, 0-9, and hyphens"}
		}
	}

	return nil
}

func Validate(skill *Skill) error {
	if skill == nil {
		return &ValidationError{Message: "skill is nil"}
	}

	if err := ValidateName(skill.Name); err != nil {
		return err
	}

	if skill.Description == "" {
		return &ValidationError{Field: "description", Message: "description is required"}
	}
	if len(skill.Description) > 1024 {
		return &ValidationError{Field: "description", Message: "description must be at most 1024 characters"}
	}

	if len(skill.Compatibility) > 500 {
		return &ValidationError{Field: "compatibility", Message: "compatibility must be at most 500 characters"}
	}

	return nil
}

func ValidateNameMatchesDir(name string, dir string) error {
	base := filepath.Base(dir)
	if base != name {
		return &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("skill name %q does not match directory %q", name, base),
		}
	}
	return nil
}
