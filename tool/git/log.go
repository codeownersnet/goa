package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type logArgs struct {
	WorkDir string   `json:"workdir,omitempty"`
	Count   int      `json:"count,omitempty"`
	Oneline bool     `json:"oneline,omitempty"`
	Branch  string   `json:"branch,omitempty"`
	Author  string   `json:"author,omitempty"`
	Since   string   `json:"since,omitempty"`
	Until   string   `json:"until,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	Format  string   `json:"format,omitempty"`
}

func newLogTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_log",
		Description: "Shows commit history. Count limits number of commits (default 20). Oneline shows one line per commit. Branch filters to a specific branch. Author filters by author. Since/until filter by date (e.g. '2 weeks ago'). Paths filters to commits touching given paths. Format sets custom format string.",
	}, func(ctx context.Context, args logArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		gitArgs := []string{"log"}

		count := args.Count
		if count <= 0 {
			count = 20
		}
		gitArgs = append(gitArgs, fmt.Sprintf("-%d", count))

		if args.Oneline {
			gitArgs = append(gitArgs, "--oneline")
		}
		if args.Branch != "" {
			gitArgs = append(gitArgs, args.Branch)
		}
		if args.Author != "" {
			gitArgs = append(gitArgs, fmt.Sprintf("--author=%s", args.Author))
		}
		if args.Since != "" {
			gitArgs = append(gitArgs, fmt.Sprintf("--since=%s", args.Since))
		}
		if args.Until != "" {
			gitArgs = append(gitArgs, fmt.Sprintf("--until=%s", args.Until))
		}
		if args.Format != "" {
			gitArgs = append(gitArgs, fmt.Sprintf("--format=%s", args.Format))
		}
		if len(args.Paths) > 0 {
			gitArgs = append(gitArgs, "--")
			gitArgs = append(gitArgs, args.Paths...)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_log: %w", err)
		}
		return resultToMap(res), nil
	})
}
