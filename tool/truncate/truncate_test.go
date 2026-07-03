package truncate

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUnderLimit(t *testing.T) {
	output := "hello\nworld"
	result := Truncate(output, Options{})
	if result.Truncated {
		t.Error("should not truncate small output")
	}
	if result.Content != output {
		t.Errorf("content mismatch: got %q, want %q", result.Content, output)
	}
}

func TestTruncateOverLineLimit(t *testing.T) {
	var lines []string
	for i := 0; i < 3000; i++ {
		lines = append(lines, "line")
	}
	output := strings.Join(lines, "\n")
	result := Truncate(output, Options{MaxLines: 2000, MaxBytes: 1024 * 1024})
	if !result.Truncated {
		t.Error("should truncate large output")
	}
	if result.OutputPath == "" {
		t.Error("should have output path")
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Error("should mention truncation")
	}
}

func TestTruncateOverByteLimit(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100000; i++ {
		sb.WriteString("x")
	}
	output := sb.String()
	result := Truncate(output, Options{MaxLines: 100000, MaxBytes: 100})
	if !result.Truncated {
		t.Error("should truncate large byte output")
	}
	if !strings.Contains(result.Content, "bytes truncated") {
		t.Error("should mention bytes truncation")
	}
}

func TestTruncateTailDirection(t *testing.T) {
	var lines []string
	for i := 0; i < 3000; i++ {
		lines = append(lines, "line")
	}
	output := strings.Join(lines, "\n")
	result := Truncate(output, Options{MaxLines: 10, MaxBytes: 1024 * 1024, Direction: "tail"})
	if !result.Truncated {
		t.Error("should truncate")
	}
	if !strings.HasPrefix(result.Content, "...") {
		t.Errorf("tail direction should start with ..., got: %s", result.Content[:50])
	}
}

func TestTruncateString(t *testing.T) {
	s := "hello world"
	result := TruncateString(s, 5)
	if result == s {
		t.Error("should truncate")
	}
	if !strings.Contains(result, "truncated") {
		t.Error("should mention truncation")
	}
}

func TestTruncateStringUnderLimit(t *testing.T) {
	s := "hello"
	result := TruncateString(s, 100)
	if result != s {
		t.Errorf("should not truncate, got %q", result)
	}
}

func TestTruncateStringUTF8(t *testing.T) {
	s := "héllo wörld"
	result := TruncateString(s, 5)
	if !utf8.ValidString(result) {
		t.Error("result should be valid UTF-8")
	}
}
