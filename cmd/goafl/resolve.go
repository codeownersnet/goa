package main

import (
	"os"
	"path/filepath"
)

func resolveWorkflowPath(arg string) string {
	if _, err := os.Stat(arg); err == nil {
		return arg
	}

	dirs := []string{
		".goa/workflows",
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".goa", "workflows"))
	}

	for _, dir := range dirs {
		for _, ext := range []string{".yaml", ".yml"} {
			candidate := filepath.Join(dir, arg+ext)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	return arg
}
