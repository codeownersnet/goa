package readfile

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type Config struct {
	AllowedPaths []string
	MaxBytes     int64
}

type readFileArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func New(cfg Config) (tool.Tool, error) {
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	guard := pathguard.New(cfg.AllowedPaths)

	return functiontool.New(functiontool.Config{
		Name:        "read_file",
		Description: "Reads the contents of a file at the given path. Returns file content, optionally starting at a line offset with a line limit.",
	}, func(_ context.Context, args readFileArgs) (map[string]any, error) {
		if err := guard.Check(args.Path); err != nil {
			return nil, fmt.Errorf("read_file: %w", err)
		}

		info, err := os.Stat(args.Path)
		if err != nil {
			return nil, fmt.Errorf("read_file: %w", err)
		}
		if info.Size() > maxBytes {
			return nil, fmt.Errorf("read_file: file size %d exceeds max %d bytes", info.Size(), maxBytes)
		}

		f, err := os.Open(args.Path)
		if err != nil {
			return nil, fmt.Errorf("read_file: %w", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		var lines []string
		var totalLines int
		lineNum := 0

		for scanner.Scan() {
			totalLines++
			lineNum++
			if args.Offset > 0 && lineNum < args.Offset {
				continue
			}
			if args.Limit > 0 && len(lines) >= args.Limit {
				continue
			}
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read_file: %w", err)
		}

		var content strings.Builder
		for i, l := range lines {
			if i > 0 {
				content.WriteByte('\n')
			}
			fmt.Fprintf(&content, "%d: %s", args.Offset+i, l)
		}
		if args.Offset <= 0 || args.Offset == 1 {
			if totalLines > len(lines) {
				fmt.Fprintf(&content, "\n(End of file - total %d lines)", totalLines)
			}
		} else {
			fmt.Fprintf(&content, "\n(Lines %d-%d of %d total)", args.Offset, args.Offset+len(lines)-1, totalLines)
		}

		return map[string]any{
			"content": content.String(),
			"lines":   totalLines,
		}, nil
	})
}
