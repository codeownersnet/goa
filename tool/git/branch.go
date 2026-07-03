package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type branchArgs struct {
	WorkDir    string `json:"workdir,omitempty"`
	Action     string `json:"action"`
	Name       string `json:"name,omitempty"`
	StartPoint string `json:"start_point,omitempty"`
	Remote     bool   `json:"remote,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

func newBranchTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_branch",
		Description: "Manages branches. Actions: create (new branch from start_point, also switches to it), list (show branches, remote for remote-tracking), switch (checkout existing branch), delete (delete a branch, force for unmerged), current (show current branch name).",
	}, func(ctx context.Context, args branchArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		var gitArgs []string
		switch args.Action {
		case "create":
			gitArgs = []string{"checkout", "-b", args.Name}
			if args.StartPoint != "" {
				gitArgs = append(gitArgs, args.StartPoint)
			}
		case "list":
			gitArgs = []string{"branch"}
			if args.Remote {
				gitArgs = append(gitArgs, "-r")
			}
		case "switch":
			gitArgs = []string{"checkout", args.Name}
		case "delete":
			if args.Force {
				gitArgs = []string{"branch", "-D", args.Name}
			} else {
				gitArgs = []string{"branch", "-d", args.Name}
			}
		case "current":
			gitArgs = []string{"branch", "--show-current"}
		default:
			return nil, fmt.Errorf("git_branch: unknown action %q, use one of: create, list, switch, delete, current", args.Action)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_branch: %w", err)
		}
		return resultToMap(res), nil
	})
}
