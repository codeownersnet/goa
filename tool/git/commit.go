package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type commitArgs struct {
	WorkDir    string `json:"workdir,omitempty"`
	Message    string `json:"message"`
	Amend      bool   `json:"amend,omitempty"`
	AllowEmpty bool   `json:"allow_empty,omitempty"`
}

func newCommitTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_commit",
		Description: "Commits staged changes with the given message. Use git_add to stage files before committing. Use amend to modify the last commit. Use allow_empty to create an empty commit.",
	}, func(ctx context.Context, args commitArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		gitArgs := []string{"commit", "-m", args.Message}
		if args.Amend {
			gitArgs = append(gitArgs, "--amend")
		}
		if args.AllowEmpty {
			gitArgs = append(gitArgs, "--allow-empty")
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_commit: %w", err)
		}
		return resultToMap(res), nil
	})
}
