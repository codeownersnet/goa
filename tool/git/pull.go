package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type pullArgs struct {
	WorkDir string `json:"workdir,omitempty"`
	Remote  string `json:"remote,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Rebase  bool   `json:"rebase,omitempty"`
	FFOnly  bool   `json:"ff_only,omitempty"`
}

func newPullTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_pull",
		Description: "Fetches from and integrates with another repository or a local branch. Use rebase for rebase strategy, ff_only to fail if fast-forward not possible.",
	}, func(ctx context.Context, args pullArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		gitArgs := []string{"pull"}
		if args.Rebase {
			gitArgs = append(gitArgs, "--rebase")
		}
		if args.FFOnly {
			gitArgs = append(gitArgs, "--ff-only")
		}
		if args.Remote != "" {
			gitArgs = append(gitArgs, args.Remote)
		}
		if args.Branch != "" {
			gitArgs = append(gitArgs, args.Branch)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_pull: %w", err)
		}
		return resultToMap(res), nil
	})
}
