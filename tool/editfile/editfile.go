package editfile

import (
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
}

type editFileArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func New(cfg Config) (tool.Tool, error) {
	guard := pathguard.New(cfg.AllowedPaths)

	return functiontool.New(functiontool.Config{
		Name:        "edit_file",
		Description: "Applies a string replacement to a file. Replaces old_string with new_string. Fails if old_string is not found or found multiple times unless replace_all is true.",
	}, func(_ context.Context, args editFileArgs) (map[string]any, error) {
		if err := guard.Check(args.Path); err != nil {
			return nil, fmt.Errorf("edit_file: %w", err)
		}

		data, err := os.ReadFile(args.Path)
		if err != nil {
			return nil, fmt.Errorf("edit_file: %w", err)
		}

		if args.OldString == args.NewString {
			return nil, fmt.Errorf("edit_file: no changes to apply, old_string and new_string are identical")
		}

		if args.OldString == "" {
			newContent := ensureTrailingNewline(args.NewString)
			if err := os.WriteFile(args.Path, []byte(newContent), 0o644); err != nil {
				return nil, fmt.Errorf("edit_file: %w", err)
			}
			return map[string]any{
				"success":      true,
				"replacements": 1,
				"message":      "Edit applied successfully.",
			}, nil
		}

		content := string(data)
		ending := detectLineEnding(content)
		old := convertToLineEnding(normalizeLineEndings(args.OldString), ending)
		newStr := convertToLineEnding(normalizeLineEndings(args.NewString), ending)

		newContent, err := fuzzyReplace(content, old, newStr, args.ReplaceAll)
		if err != nil {
			return nil, fmt.Errorf("edit_file: %w", err)
		}

		newContent = ensureTrailingNewline(newContent)

		if err := os.WriteFile(args.Path, []byte(newContent), 0o644); err != nil {
			return nil, fmt.Errorf("edit_file: %w", err)
		}

		return map[string]any{
			"success": true,
			"message": "Edit applied successfully.",
		}, nil
	})
}

func ensureTrailingNewline(s string) string {
	if len(s) == 0 || s[len(s)-1] != '\n' {
		return s + "\n"
	}
	return s
}

func normalizeLineEndings(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}

func detectLineEnding(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func convertToLineEnding(text, ending string) string {
	if ending == "\n" {
		return text
	}
	return strings.ReplaceAll(text, "\n", "\r\n")
}

func fuzzyReplace(content, oldString, newString string, replaceAll bool) (string, error) {
	notFound := true

	for _, r := range replacers {
		for search := range r(content, oldString) {
			idx := strings.Index(content, search)
			if idx == -1 {
				continue
			}
			notFound = false
			if replaceAll {
				return strings.ReplaceAll(content, search, newString), nil
			}
			lastIdx := strings.LastIndex(content, search)
			if idx != lastIdx {
				continue
			}
			return content[:idx] + newString + content[idx+len(search):], nil
		}
	}

	if notFound {
		return "", fmt.Errorf("could not find old_string in the file. It must match exactly, including whitespace, indentation, and line endings")
	}
	return "", fmt.Errorf("found multiple matches for old_string. Provide more surrounding context to make the match unique")
}
