package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type pushArgs struct {
	WorkDir     string `json:"workdir,omitempty"`
	Remote      string `json:"remote,omitempty"`
	Branch      string `json:"branch,omitempty"`
	SetUpstream bool   `json:"set_upstream,omitempty"`
	Force       bool   `json:"force,omitempty"`
	Tags        bool   `json:"tags,omitempty"`
}

func newPushTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_push",
		Description: "Pushes local commits to a remote repository. Use set_upstream to track a new remote branch. Force overwrites remote history — use with caution. Tags pushes all tags.",
	}, func(ctx context.Context, args pushArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		gitArgs := []string{"push"}
		if args.SetUpstream {
			gitArgs = append(gitArgs, "-u")
		}
		if args.Force {
			gitArgs = append(gitArgs, "--force")
		}
		if args.Tags {
			gitArgs = append(gitArgs, "--tags")
		}
		if args.Remote != "" {
			gitArgs = append(gitArgs, args.Remote)
		}
		if args.Branch != "" {
			gitArgs = append(gitArgs, args.Branch)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_push: %w", err)
		}
		return resultToMap(res), nil
	})
}
