package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type workflowInfo struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func listCmd(_ []string) {
	dirs := []string{
		".goa/workflows",
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".goa", "workflows"))
	}

	var uniqueDirs []string
	seen := make(map[string]bool)
	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if !seen[abs] {
			seen[abs] = true
			uniqueDirs = append(uniqueDirs, abs)
		}
	}

	listed := 0
	for _, dir := range uniqueDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !(filepath.Ext(entry.Name()) == ".yaml" || filepath.Ext(entry.Name()) == ".yml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			var info workflowInfo
			if err := yaml.Unmarshal(data, &info); err != nil {
				continue
			}
			if info.Name == "" {
				continue
			}
			fmt.Printf("%-30s %s\n", info.Name, info.Description)
			listed++
		}
	}

	if listed == 0 {
		fmt.Fprintln(os.Stderr, "no workflows found")
		os.Exit(1)
	}
}
