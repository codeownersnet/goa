package editfile

import (
	"regexp"
	"strings"
)

type replacer func(content, find string) func(yield func(string) bool)

func simpleReplacer(_ string, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		yield(find)
	}
}

func lineTrimmedReplacer(content, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		originalLines := strings.Split(content, "\n")
		searchLines := strings.Split(find, "\n")
		if len(searchLines) > 0 && searchLines[len(searchLines)-1] == "" {
			searchLines = searchLines[:len(searchLines)-1]
		}
		if len(searchLines) == 0 {
			return
		}
		for i := 0; i <= len(originalLines)-len(searchLines); i++ {
			match := true
			for j := 0; j < len(searchLines); j++ {
				if strings.TrimSpace(originalLines[i+j]) != strings.TrimSpace(searchLines[j]) {
					match = false
					break
				}
			}
			if match {
				start := 0
				for k := 0; k < i; k++ {
					start += len(originalLines[k]) + 1
				}
				end := start
				for k := 0; k < len(searchLines); k++ {
					end += len(originalLines[i+k])
					if k < len(searchLines)-1 {
						end++
					}
				}
				if !yield(content[start:end]) {
					return
				}
			}
		}
	}
}

func blockAnchorReplacer(content, find string) func(yield func(string) bool) {
	const singleCandidateThreshold = 0.0
	const multipleCandidatesThreshold = 0.3

	return func(yield func(string) bool) {
		originalLines := strings.Split(content, "\n")
		searchLines := strings.Split(find, "\n")
		if len(searchLines) > 0 && searchLines[len(searchLines)-1] == "" {
			searchLines = searchLines[:len(searchLines)-1]
		}
		if len(searchLines) < 3 {
			return
		}

		firstLineSearch := strings.TrimSpace(searchLines[0])
		lastLineSearch := strings.TrimSpace(searchLines[len(searchLines)-1])

		type candidate struct {
			startLine, endLine int
		}
		var candidates []candidate

		for i := 0; i < len(originalLines); i++ {
			if strings.TrimSpace(originalLines[i]) != firstLineSearch {
				continue
			}
			for j := i + 2; j < len(originalLines); j++ {
				if strings.TrimSpace(originalLines[j]) == lastLineSearch {
					candidates = append(candidates, candidate{i, j})
					break
				}
			}
		}

		if len(candidates) == 0 {
			return
		}

		searchBlockSize := len(searchLines)

		calcSimilarity := func(c candidate) float64 {
			actualBlockSize := c.endLine - c.startLine + 1
			linesToCheck := searchBlockSize - 2
			if actualBlockSize-2 < linesToCheck {
				linesToCheck = actualBlockSize - 2
			}
			if linesToCheck <= 0 {
				return 1.0
			}
			var similarity float64
			for j := 1; j < searchBlockSize-1 && j < actualBlockSize-1; j++ {
				originalLine := strings.TrimSpace(originalLines[c.startLine+j])
				searchLine := strings.TrimSpace(searchLines[j])
				maxLen := len(originalLine)
				if len(searchLine) > maxLen {
					maxLen = len(searchLine)
				}
				if maxLen == 0 {
					continue
				}
				dist := levenshtein(originalLine, searchLine)
				similarity += (1.0 - float64(dist)/float64(maxLen)) / float64(linesToCheck)
			}
			return similarity
		}

		extractMatch := func(c candidate) string {
			start := 0
			for k := 0; k < c.startLine; k++ {
				start += len(originalLines[k]) + 1
			}
			end := start
			for k := c.startLine; k <= c.endLine; k++ {
				end += len(originalLines[k])
				if k < c.endLine {
					end++
				}
			}
			return content[start:end]
		}

		if len(candidates) == 1 {
			if calcSimilarity(candidates[0]) >= singleCandidateThreshold {
				yield(extractMatch(candidates[0]))
			}
			return
		}

		var bestMatch *candidate
		var maxSimilarity float64 = -1
		for i := range candidates {
			s := calcSimilarity(candidates[i])
			if s > maxSimilarity {
				maxSimilarity = s
				c := candidates[i]
				bestMatch = &c
			}
		}
		if maxSimilarity >= multipleCandidatesThreshold && bestMatch != nil {
			yield(extractMatch(*bestMatch))
		}
	}
}

func whitespaceNormalizedReplacer(content, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		normalize := func(text string) string {
			var b strings.Builder
			b.Grow(len(text))
			prevSpace := false
			for _, r := range text {
				if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
					if !prevSpace {
						b.WriteByte(' ')
						prevSpace = true
					}
				} else {
					b.WriteRune(r)
					prevSpace = false
				}
			}
			return strings.TrimSpace(b.String())
		}

		normalizedFind := normalize(find)
		lines := strings.Split(content, "\n")

		for _, line := range lines {
			if normalize(line) == normalizedFind {
				if !yield(line) {
					return
				}
			} else {
				normalizedLine := normalize(line)
				if strings.Contains(normalizedLine, normalizedFind) {
					words := strings.Fields(find)
					if len(words) > 0 {
						var pattern strings.Builder
						for i, w := range words {
							if i > 0 {
								pattern.WriteString(`\s+`)
							}
							pattern.WriteString(regexp.QuoteMeta(w))
						}
						re, err := regexp.Compile(pattern.String())
						if err == nil {
							if loc := re.FindStringIndex(line); loc != nil {
								if !yield(line[loc[0]:loc[1]]) {
									return
								}
							}
						}
					}
				}
			}
		}

		findLines := strings.Split(find, "\n")
		if len(findLines) > 1 {
			for i := 0; i <= len(lines)-len(findLines); i++ {
				block := strings.Join(lines[i:i+len(findLines)], "\n")
				if normalize(block) == normalizedFind {
					if !yield(block) {
						return
					}
				}
			}
		}
	}
}

func indentationFlexibleReplacer(content, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		removeIndentation := func(text string) string {
			lines := strings.Split(text, "\n")
			nonEmpty := make([]string, 0, len(lines))
			for _, l := range lines {
				if strings.TrimSpace(l) != "" {
					nonEmpty = append(nonEmpty, l)
				}
			}
			if len(nonEmpty) == 0 {
				return text
			}
			minIndent := len(nonEmpty[0])
			for _, l := range nonEmpty {
				leading := 0
				for _, r := range l {
					if r == ' ' || r == '\t' {
						leading++
					} else {
						break
					}
				}
				if leading < minIndent {
					minIndent = leading
				}
			}
			result := make([]string, len(lines))
			for i, l := range lines {
				if strings.TrimSpace(l) == "" {
					result[i] = l
				} else if len(l) > minIndent {
					result[i] = l[minIndent:]
				} else {
					result[i] = l
				}
			}
			return strings.Join(result, "\n")
		}

		normalizedFind := removeIndentation(find)
		contentLines := strings.Split(content, "\n")
		findLines := strings.Split(find, "\n")

		for i := 0; i <= len(contentLines)-len(findLines); i++ {
			block := strings.Join(contentLines[i:i+len(findLines)], "\n")
			if removeIndentation(block) == normalizedFind {
				if !yield(block) {
					return
				}
			}
		}
	}
}

func escapeNormalizedReplacer(content, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		unescape := func(s string) string {
			var b strings.Builder
			b.Grow(len(s))
			i := 0
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					switch s[i+1] {
					case 'n':
						b.WriteByte('\n')
						i += 2
						continue
					case 't':
						b.WriteByte('\t')
						i += 2
						continue
					case 'r':
						b.WriteByte('\r')
						i += 2
						continue
					case '\\':
						b.WriteByte('\\')
						i += 2
						continue
					case '\'':
						b.WriteByte('\'')
						i += 2
						continue
					case '"':
						b.WriteByte('"')
						i += 2
						continue
					case '`':
						b.WriteByte('`')
						i += 2
						continue
					case '$':
						b.WriteByte('$')
						i += 2
						continue
					}
				}
				b.WriteByte(s[i])
				i++
			}
			return b.String()
		}

		unescapedFind := unescape(find)

		if strings.Contains(content, unescapedFind) {
			if !yield(unescapedFind) {
				return
			}
		}

		lines := strings.Split(content, "\n")
		findLines := strings.Split(unescapedFind, "\n")

		for i := 0; i <= len(lines)-len(findLines); i++ {
			block := strings.Join(lines[i:i+len(findLines)], "\n")
			if unescape(block) == unescapedFind {
				if !yield(block) {
					return
				}
			}
		}
	}
}

func trimmedBoundaryReplacer(content, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		trimmedFind := strings.TrimSpace(find)
		if trimmedFind == find {
			return
		}

		if strings.Contains(content, trimmedFind) {
			if !yield(trimmedFind) {
				return
			}
		}

		lines := strings.Split(content, "\n")
		findLines := strings.Split(find, "\n")

		for i := 0; i <= len(lines)-len(findLines); i++ {
			block := strings.Join(lines[i:i+len(findLines)], "\n")
			if strings.TrimSpace(block) == trimmedFind {
				if !yield(block) {
					return
				}
			}
		}
	}
}

func contextAwareReplacer(content, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		findLines := strings.Split(find, "\n")
		if len(findLines) < 3 {
			return
		}
		if findLines[len(findLines)-1] == "" {
			findLines = findLines[:len(findLines)-1]
		}
		if len(findLines) < 3 {
			return
		}

		contentLines := strings.Split(content, "\n")
		firstLine := strings.TrimSpace(findLines[0])
		lastLine := strings.TrimSpace(findLines[len(findLines)-1])

		for i := 0; i < len(contentLines); i++ {
			if strings.TrimSpace(contentLines[i]) != firstLine {
				continue
			}
			for j := i + 2; j < len(contentLines); j++ {
				if strings.TrimSpace(contentLines[j]) != lastLine {
					continue
				}
				blockLines := contentLines[i : j+1]
				if len(blockLines) != len(findLines) {
					break
				}
				var matchingLines, totalNonEmpty int
				for k := 1; k < len(blockLines)-1; k++ {
					bl := strings.TrimSpace(blockLines[k])
					fl := strings.TrimSpace(findLines[k])
					if len(bl) > 0 || len(fl) > 0 {
						totalNonEmpty++
						if bl == fl {
							matchingLines++
						}
					}
				}
				if totalNonEmpty == 0 || float64(matchingLines)/float64(totalNonEmpty) >= 0.5 {
					block := strings.Join(blockLines, "\n")
					yield(block)
					return
				}
				break
			}
		}
	}
}

func multiOccurrenceReplacer(content, find string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		start := 0
		for {
			idx := strings.Index(content[start:], find)
			if idx == -1 {
				return
			}
			if !yield(find) {
				return
			}
			start += idx + len(find)
		}
	}
}

var replacers = []replacer{
	simpleReplacer,
	lineTrimmedReplacer,
	blockAnchorReplacer,
	whitespaceNormalizedReplacer,
	indentationFlexibleReplacer,
	escapeNormalizedReplacer,
	trimmedBoundaryReplacer,
	contextAwareReplacer,
	multiOccurrenceReplacer,
}
