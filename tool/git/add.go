package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type addArgs struct {
	WorkDir string   `json:"workdir,omitempty"`
	Paths   []string `json:"paths"`
	All     bool     `json:"all,omitempty"`
	Update  bool     `json:"update,omitempty"`
	Patch   bool     `json:"patch,omitempty"`
}

func newAddTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_add",
		Description: "Stages file changes for the next commit. Use all to stage all changes including untracked (git add -A). Use update to stage only tracked files (git add -u). Use patch for interactive patch selection. Default: stages the specified paths.",
	}, func(ctx context.Context, args addArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		var gitArgs []string
		switch {
		case args.All:
			gitArgs = []string{"add", "-A"}
		case args.Update:
			gitArgs = []string{"add", "-u"}
		case args.Patch:
			gitArgs = []string{"add", "-p"}
			gitArgs = append(gitArgs, args.Paths...)
		default:
			gitArgs = []string{"add"}
			gitArgs = append(gitArgs, args.Paths...)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_add: %w", err)
		}
		return resultToMap(res), nil
	})
}
