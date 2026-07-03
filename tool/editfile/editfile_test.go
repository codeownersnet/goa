package editfile

import "testing"

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"hello", "world", 4},
	}
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSimpleReplacer(t *testing.T) {
	content := "hello world"
	found := false
	for match := range simpleReplacer(content, "hello") {
		if match != "hello" {
			t.Errorf("simpleReplacer: got %q, want %q", match, "hello")
		}
		found = true
	}
	if !found {
		t.Error("simpleReplacer: expected yield")
	}
}

func TestLineTrimmedReplacer(t *testing.T) {
	content := "  hello  \n  world  \n  foo  "
	find := "hello\nworld"
	found := false
	for match := range lineTrimmedReplacer(content, find) {
		if match != "  hello  \n  world  " {
			t.Errorf("lineTrimmedReplacer: got %q", match)
		}
		found = true
	}
	if !found {
		t.Error("lineTrimmedReplacer: expected yield")
	}
}

func TestBlockAnchorReplacer(t *testing.T) {
	content := "func main() {\n\tfmt.Println(\"hello\")\n\treturn\n}"
	find := "func main() {\n\tfmt.Println(\"hi\")\n\treturn\n}"
	found := false
	for match := range blockAnchorReplacer(content, find) {
		if match != "func main() {\n\tfmt.Println(\"hello\")\n\treturn\n}" {
			t.Errorf("blockAnchorReplacer: got %q", match)
		}
		found = true
	}
	if !found {
		t.Error("blockAnchorReplacer: expected yield")
	}
}

func TestBlockAnchorReplacerTooShort(t *testing.T) {
	content := "ab"
	find := "ab"
	found := false
	for range blockAnchorReplacer(content, find) {
		found = true
	}
	if found {
		t.Error("blockAnchorReplacer: should not yield for <3 lines")
	}
}

func TestWhitespaceNormalizedReplacer(t *testing.T) {
	content := "  hello   world  "
	find := "hello world"
	found := false
	for match := range whitespaceNormalizedReplacer(content, find) {
		if match != "  hello   world  " {
			t.Errorf("whitespaceNormalizedReplacer: got %q", match)
		}
		found = true
	}
	if !found {
		t.Error("whitespaceNormalizedReplacer: expected yield")
	}
}

func TestIndentationFlexibleReplacer(t *testing.T) {
	content := "\thello\n\t\tworld\n\tfoo"
	find := "hello\n\tworld\nfoo"
	found := false
	for match := range indentationFlexibleReplacer(content, find) {
		if match != "\thello\n\t\tworld\n\tfoo" {
			t.Errorf("indentationFlexibleReplacer: got %q", match)
		}
		found = true
	}
	if !found {
		t.Error("indentationFlexibleReplacer: expected yield")
	}
}

func TestEscapeNormalizedReplacer(t *testing.T) {
	content := "hello\nworld"
	find := "hello\\nworld"
	found := false
	for match := range escapeNormalizedReplacer(content, find) {
		if match != "hello\nworld" {
			t.Errorf("escapeNormalizedReplacer: got %q", match)
		}
		found = true
	}
	if !found {
		t.Error("escapeNormalizedReplacer: expected yield")
	}
}

func TestTrimmedBoundaryReplacer(t *testing.T) {
	content := "hello world"
	find := "  hello world  "
	found := false
	for match := range trimmedBoundaryReplacer(content, find) {
		if match != "hello world" {
			t.Errorf("trimmedBoundaryReplacer: got %q", match)
		}
		found = true
	}
	if !found {
		t.Error("trimmedBoundaryReplacer: expected yield")
	}
}

func TestContextAwareReplacer(t *testing.T) {
	content := "func main() {\n\tfmt.Println()\n\treturn\n}"
	find := "func main() {\n\tfmt.Println(\"hi\")\n\treturn\n}"
	found := false
	for match := range contextAwareReplacer(content, find) {
		if match != "func main() {\n\tfmt.Println()\n\treturn\n}" {
			t.Errorf("contextAwareReplacer: got %q", match)
		}
		found = true
	}
	if !found {
		t.Error("contextAwareReplacer: expected yield")
	}
}

func TestMultiOccurrenceReplacer(t *testing.T) {
	content := "aaa bbb aaa"
	find := "aaa"
	count := 0
	for range multiOccurrenceReplacer(content, find) {
		count++
	}
	if count != 2 {
		t.Errorf("multiOccurrenceReplacer: got %d occurrences, want 2", count)
	}
}

func TestFuzzyReplaceExact(t *testing.T) {
	content := "hello world"
	result, err := fuzzyReplace(content, "hello", "hi", false)
	if err != nil {
		t.Fatal(err)
	}
	if result != "hi world" {
		t.Errorf("fuzzyReplace: got %q, want %q", result, "hi world")
	}
}

func TestFuzzyReplaceNotFound(t *testing.T) {
	content := "hello world"
	_, err := fuzzyReplace(content, "xyz", "abc", false)
	if err == nil {
		t.Error("fuzzyReplace: expected error for not found")
	}
}

func TestFuzzyReplaceMultipleMatches(t *testing.T) {
	content := "aaa bbb aaa"
	_, err := fuzzyReplace(content, "aaa", "ccc", false)
	if err == nil {
		t.Error("fuzzyReplace: expected error for multiple matches")
	}
}

func TestFuzzyReplaceReplaceAll(t *testing.T) {
	content := "aaa bbb aaa"
	result, err := fuzzyReplace(content, "aaa", "ccc", true)
	if err != nil {
		t.Fatal(err)
	}
	if result != "ccc bbb ccc" {
		t.Errorf("fuzzyReplace replaceAll: got %q, want %q", result, "ccc bbb ccc")
	}
}

func TestFuzzyReplaceWhitespaceMismatch(t *testing.T) {
	content := "  hello  \n  world  "
	result, err := fuzzyReplace(content, "hello\nworld", "hi\nearth", false)
	if err != nil {
		t.Fatal(err)
	}
	want := "hi\nearth"
	if result != want {
		t.Errorf("fuzzyReplace whitespace: got %q, want %q", result, want)
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	if normalizeLineEndings("a\r\nb\r\n") != "a\nb\n" {
		t.Error("normalizeLineEndings failed")
	}
}

func TestDetectLineEnding(t *testing.T) {
	if detectLineEnding("a\nb\n") != "\n" {
		t.Error("detectLineEnding \\n failed")
	}
	if detectLineEnding("a\r\nb\r\n") != "\r\n" {
		t.Error("detectLineEnding \\r\\n failed")
	}
}

func TestConvertToLineEnding(t *testing.T) {
	if convertToLineEnding("a\nb\n", "\n") != "a\nb\n" {
		t.Error("convertToLineEnding \\n failed")
	}
	if convertToLineEnding("a\nb\n", "\r\n") != "a\r\nb\r\n" {
		t.Error("convertToLineEnding \\r\\n failed")
	}
}

func TestEnsureTrailingNewline(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello\n"},
		{"hello\n", "hello\n"},
		{"", "\n"},
		{"a\nb", "a\nb\n"},
		{"a\nb\n", "a\nb\n"},
	}
	for _, tt := range tests {
		got := ensureTrailingNewline(tt.input)
		if got != tt.want {
			t.Errorf("ensureTrailingNewline(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
