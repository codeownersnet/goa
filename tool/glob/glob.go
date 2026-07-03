package glob

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type Config struct {
	AllowedPaths []string
	MaxResults   int
}

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func New(cfg Config) (tool.Tool, error) {
	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	guard := pathguard.New(cfg.AllowedPaths)

	return functiontool.New(functiontool.Config{
		Name:        "glob",
		Description: "Finds files matching a glob pattern (e.g. **/*.go) in a directory. Returns matching file paths.",
	}, func(_ context.Context, args globArgs) (map[string]any, error) {
		if err := guard.Check(args.Path); err != nil {
			return nil, fmt.Errorf("glob: %w", err)
		}

		pattern := filepath.Join(args.Path, args.Pattern)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob: invalid pattern: %w", err)
		}

		if len(matches) > maxResults {
			matches = matches[:maxResults]
		}

		return map[string]any{"files": matches}, nil
	})
}
