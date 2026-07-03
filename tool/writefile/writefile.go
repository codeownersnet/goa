package writefile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type Config struct {
	AllowedPaths []string
	MaxBytes     int64
	CreateDirs   bool
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func New(cfg Config) (tool.Tool, error) {
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 10 << 20
	}
	guard := pathguard.New(cfg.AllowedPaths)

	return functiontool.New(functiontool.Config{
		Name:        "write_file",
		Description: "Creates or overwrites a file with the given content. Optionally creates parent directories.",
	}, func(_ context.Context, args writeFileArgs) (map[string]any, error) {
		if err := guard.Check(args.Path); err != nil {
			return nil, fmt.Errorf("write_file: %w", err)
		}

		if int64(len(args.Content)) > maxBytes {
			return nil, fmt.Errorf("write_file: content size %d exceeds max %d bytes", len(args.Content), maxBytes)
		}

		if cfg.CreateDirs {
			dir := filepath.Dir(args.Path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("write_file: create directories: %w", err)
			}
		}

		content := args.Content
		if len(content) == 0 || content[len(content)-1] != '\n' {
			content += "\n"
		}

		if err := os.WriteFile(args.Path, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("write_file: %w", err)
		}

		return map[string]any{
			"success":       true,
			"bytes_written": len(content),
			"message":       "File written successfully.",
		}, nil
	})
}
