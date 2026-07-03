package truncate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxLines = 2000
	MaxBytes = 50 * 1024
)

var truncationDir = func() string {
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, ".local", "share", "goa", "tool-output")
	}
	return filepath.Join(os.TempDir(), "goa", "tool-output")
}()

type Options struct {
	MaxLines  int
	MaxBytes  int
	Direction string
}

type Result struct {
	Content    string
	Truncated  bool
	OutputPath string
}

func Truncate(output string, opts Options) Result {
	Cleanup(0)

	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = MaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxBytes
	}
	direction := opts.Direction
	if direction == "" {
		direction = "head"
	}

	lines := strings.Split(output, "\n")
	totalBytes := len(output)

	if len(lines) <= maxLines && totalBytes <= maxBytes {
		return Result{Content: output, Truncated: false}
	}

	var out []string
	bytes := 0
	hitBytes := false

	if direction == "head" {
		for i := 0; i < len(lines) && i < maxLines; i++ {
			size := len(lines[i])
			if i > 0 {
				size++
			}
			if bytes+size > maxBytes {
				hitBytes = true
				break
			}
			out = append(out, lines[i])
			bytes += size
		}
	} else {
		for i := len(lines) - 1; i >= 0 && len(out) < maxLines; i-- {
			size := len(lines[i])
			if len(out) > 0 {
				size++
			}
			if bytes+size > maxBytes {
				hitBytes = true
				break
			}
			out = append([]string{lines[i]}, out...)
			bytes += size
		}
	}

	removed := totalBytes - bytes
	if !hitBytes {
		removed = len(lines) - len(out)
	}
	unit := "lines"
	if hitBytes {
		unit = "bytes"
	}

	preview := strings.Join(out, "\n")
	outputPath := filepath.Join(truncationDir, fmt.Sprintf("tool_%d", time.Now().UnixNano()))

	hint := "The tool call succeeded but the output was truncated."
	if err := os.MkdirAll(truncationDir, 0o755); err == nil {
		if err := os.WriteFile(outputPath, []byte(output), 0o644); err == nil {
			hint = fmt.Sprintf(
				"%s Full output saved to: %s\nUse Grep to search the full content or ReadFile with offset/limit to view specific sections.",
				hint,
				outputPath,
			)
		} else {
			outputPath = ""
		}
	} else {
		outputPath = ""
	}

	var content string
	if direction == "head" {
		content = fmt.Sprintf("%s\n\n...%d %s truncated...\n\n%s", preview, removed, unit, hint)
	} else {
		content = fmt.Sprintf("...%d %s truncated...\n\n%s\n\n%s", removed, unit, hint, preview)
	}

	return Result{
		Content:    content,
		Truncated:  true,
		OutputPath: outputPath,
	}
}

func TruncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = MaxBytes
	}
	if len(s) <= maxLen {
		return s
	}
	if utf8.ValidString(s[:maxLen]) {
		return s[:maxLen] + fmt.Sprintf("\n\n...%d bytes truncated...", len(s)-maxLen)
	}
	for i := maxLen; i > 0; i-- {
		if utf8.ValidString(s[:i]) {
			return s[:i] + fmt.Sprintf("\n\n...%d bytes truncated...", len(s)-i)
		}
	}
	return s
}

func Cleanup(maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}
	entries, err := os.ReadDir(truncationDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "tool_") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(truncationDir, entry.Name()))
		}
	}
}
