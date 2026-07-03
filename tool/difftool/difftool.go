package difftool

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

type diffArgs struct {
	Path         string `json:"path"`
	NewContent   string `json:"new_content"`
	ContextLines int    `json:"context_lines,omitempty"`
}

func New(cfg Config) (tool.Tool, error) {
	guard := pathguard.New(cfg.AllowedPaths)

	return functiontool.New(functiontool.Config{
		Name:        "diff",
		Description: "Compares a file on disk against proposed new content and returns a unified diff. Does not modify the file.",
	}, func(_ context.Context, args diffArgs) (map[string]any, error) {
		if err := guard.Check(args.Path); err != nil {
			return nil, fmt.Errorf("diff: %w", err)
		}

		data, err := os.ReadFile(args.Path)
		if err != nil {
			return nil, fmt.Errorf("diff: %w", err)
		}

		oldContent := string(data)
		contextLines := args.ContextLines
		if contextLines <= 0 {
			contextLines = 3
		}

		diff := unifiedDiff(oldContent, args.NewContent, contextLines)
		hasChanges := diff != ""

		return map[string]any{
			"diff":        diff,
			"has_changes": hasChanges,
		}, nil
	})
}

func unifiedDiff(oldStr, newStr string, context int) string {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	if oldStr == newStr {
		return ""
	}

	ops := diffLines(oldLines, newLines)
	if len(ops) == 0 {
		return ""
	}

	groups := groupHunks(ops, context)
	var sb strings.Builder

	for _, group := range groups {
		oldStart, oldCount, newStart, newCount := groupBounds(group, oldLines, newLines)
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart+1, oldCount, newStart+1, newCount))

		for _, op := range group {
			switch op.kind {
			case opEqual:
				sb.WriteString(" " + oldLines[op.oldIdx] + "\n")
			case opDelete:
				sb.WriteString("-" + oldLines[op.oldIdx] + "\n")
			case opInsert:
				sb.WriteString("+" + newLines[op.newIdx] + "\n")
			}
		}
	}

	return sb.String()
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type diffOp struct {
	kind   opKind
	oldIdx int
	newIdx int
}

func diffLines(old, new []string) []diffOp {
	n := len(old)
	m := len(new)

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if old[i] == new[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				if dp[i+1][j] > dp[i][j+1] {
					dp[i][j] = dp[i+1][j]
				} else {
					dp[i][j] = dp[i][j+1]
				}
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		if old[i] == new[j] {
			ops = append(ops, diffOp{kind: opEqual, oldIdx: i, newIdx: j})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, diffOp{kind: opDelete, oldIdx: i, newIdx: j})
			i++
		} else {
			ops = append(ops, diffOp{kind: opInsert, oldIdx: i, newIdx: j})
			j++
		}
	}
	for i < n {
		ops = append(ops, diffOp{kind: opDelete, oldIdx: i, newIdx: j})
		i++
	}
	for j < m {
		ops = append(ops, diffOp{kind: opInsert, oldIdx: i, newIdx: j})
		j++
	}

	return ops
}

func groupHunks(ops []diffOp, context int) [][]diffOp {
	var changeIdxs []int
	for i, op := range ops {
		if op.kind != opEqual {
			changeIdxs = append(changeIdxs, i)
		}
	}
	if len(changeIdxs) == 0 {
		return nil
	}

	var groups [][]diffOp
	start := max(0, changeIdxs[0]-context)

	for i := 0; i < len(changeIdxs)-1; i++ {
		gap := changeIdxs[i+1] - changeIdxs[i] - 1
		if gap > 2*context {
			end := min(len(ops), changeIdxs[i]+context+1)
			groups = append(groups, ops[start:end])
			start = max(0, changeIdxs[i+1]-context)
		}
	}
	end := min(len(ops), changeIdxs[len(changeIdxs)-1]+context+1)
	groups = append(groups, ops[start:end])

	return groups
}

func groupBounds(group []diffOp, _, _ []string) (oldStart, oldCount, newStart, newCount int) {
	oldStart = -1
	newStart = -1
	oldCount = 0
	newCount = 0

	for _, op := range group {
		switch op.kind {
		case opEqual, opDelete:
			if oldStart == -1 {
				oldStart = op.oldIdx
			}
			oldCount++
		}
		switch op.kind {
		case opEqual, opInsert:
			if newStart == -1 {
				newStart = op.newIdx
			}
			newCount++
		}
	}

	return oldStart, oldCount, newStart, newCount
}
