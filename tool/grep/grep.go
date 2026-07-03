package grep

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

var errMaxResultsReached = errors.New("max grep results reached")

type Config struct {
	AllowedPaths []string
	MaxResults   int
}

type grepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Include string `json:"include,omitempty"`
}

type grepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

func New(cfg Config) (tool.Tool, error) {
	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	guard := pathguard.New(cfg.AllowedPaths)

	return functiontool.New(functiontool.Config{
		Name:        "grep",
		Description: "Searches file contents by regex pattern in a directory. Returns matching file, line number, and content. Optionally filter by file pattern (e.g. *.go).",
	}, func(_ context.Context, args grepArgs) (map[string]any, error) {
		if err := guard.Check(args.Path); err != nil {
			return nil, fmt.Errorf("grep: %w", err)
		}

		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return nil, fmt.Errorf("grep: invalid regex: %w", err)
		}

		var matches []grepMatch

		err = filepath.WalkDir(args.Path, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if args.Include != "" {
				matched, _ := filepath.Match(args.Include, filepath.Base(path))
				if !matched {
					return nil
				}
			}

			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer func() { _ = f.Close() }()

			scanner := bufio.NewScanner(f)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				if re.MatchString(line) {
					matches = append(matches, grepMatch{
						File:    path,
						Line:    lineNum,
						Content: line,
					})
					if len(matches) >= maxResults {
						return errMaxResultsReached
					}
				}
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("scan %s: %w", path, err)
			}
			return nil
		})
		if err != nil && !errors.Is(err, errMaxResultsReached) {
			return nil, fmt.Errorf("grep: %w", err)
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Found %d matches", len(matches))
		if len(matches) >= maxResults {
			fmt.Fprintf(&b, " (showing first %d)", maxResults)
		}
		b.WriteString("\n")

		currentFile := ""
		for _, m := range matches {
			if m.File != currentFile {
				currentFile = m.File
				fmt.Fprintf(&b, "%s:\n", m.File)
			}
			fmt.Fprintf(&b, "  Line %d: %s\n", m.Line, m.Content)
		}

		return map[string]any{"output": b.String(), "count": len(matches)}, nil
	})
}
