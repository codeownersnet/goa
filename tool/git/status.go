package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type statusArgs struct {
	WorkDir string `json:"workdir,omitempty"`
	Short   bool   `json:"short,omitempty"`
	Branch  bool   `json:"branch,omitempty"`
}

func newStatusTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_status",
		Description: "Shows the working tree status. Short gives porcelain output. Branch shows branch tracking info.",
	}, func(ctx context.Context, args statusArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		gitArgs := []string{"status"}
		if args.Short {
			gitArgs = append(gitArgs, "--short")
		}
		if args.Branch {
			gitArgs = append(gitArgs, "-b")
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_status: %w", err)
		}
		return resultToMap(res), nil
	})
}
