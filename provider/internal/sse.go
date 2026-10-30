package internal

import (
	"bufio"
	"io"
	"strings"
)

type SSEEvent struct {
	Data string
}

type SSEScanner struct {
	scanner *bufio.Scanner
	event   SSEEvent
	err     error
	done    bool
}

func NewSSEScanner(r io.Reader) *SSEScanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &SSEScanner{scanner: sc}
}

func (s *SSEScanner) Next() bool {
	if s.done {
		return false
	}

	var dataLines []string

	for s.scanner.Scan() {
		line := s.scanner.Text()

		if line == "" {
			if len(dataLines) > 0 {
				s.event = SSEEvent{Data: strings.Join(dataLines, "\n")}
				return true
			}
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
	}

	s.err = s.scanner.Err()
	s.done = true
	if len(dataLines) > 0 {
		s.event = SSEEvent{Data: strings.Join(dataLines, "\n")}
		return true
	}
	return false
}

func (s *SSEScanner) Event() SSEEvent { return s.event }

func (s *SSEScanner) Err() error { return s.err }
