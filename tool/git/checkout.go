package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type checkoutArgs struct {
	WorkDir   string   `json:"workdir,omitempty"`
	Action    string   `json:"action"`
	Target    string   `json:"target"`
	NewBranch bool     `json:"new_branch,omitempty"`
	Paths     []string `json:"paths,omitempty"`
}

func newCheckoutTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_checkout",
		Description: "Switches branches or restores working tree files. Actions: switch (change to a branch, new_branch to create and switch), restore (discard changes in working directory, target is a ref like HEAD or a branch, paths for specific files).",
	}, func(ctx context.Context, args checkoutArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		var gitArgs []string
		switch args.Action {
		case "switch":
			gitArgs = []string{"checkout"}
			if args.NewBranch {
				gitArgs = append(gitArgs, "-b")
			}
			gitArgs = append(gitArgs, args.Target)
		case "restore":
			gitArgs = []string{"checkout"}
			if args.Target != "" {
				gitArgs = append(gitArgs, args.Target)
			}
			if len(args.Paths) > 0 {
				gitArgs = append(gitArgs, "--")
				gitArgs = append(gitArgs, args.Paths...)
			} else {
				gitArgs = append(gitArgs, "--", ".")
			}
		default:
			return nil, fmt.Errorf("git_checkout: unknown action %q, use one of: switch, restore", args.Action)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_checkout: %w", err)
		}
		return resultToMap(res), nil
	})
}
