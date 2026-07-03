package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type stashArgs struct {
	WorkDir string `json:"workdir,omitempty"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
	Index   int    `json:"index,omitempty"`
}

func newStashTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_stash",
		Description: "Temporarily shelves changes. Actions: save (stash current changes, optional message), pop (apply and drop latest or specific stash), list (show all stashes), apply (apply latest or specific stash without dropping), drop (remove latest or specific stash).",
	}, func(ctx context.Context, args stashArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		var gitArgs []string
		switch args.Action {
		case "save":
			gitArgs = []string{"stash", "save"}
			if args.Message != "" {
				gitArgs = append(gitArgs, "-m", args.Message)
			}
		case "pop":
			gitArgs = []string{"stash", "pop"}
			if args.Index > 0 {
				gitArgs = append(gitArgs, fmt.Sprintf("stash@{%d}", args.Index))
			}
		case "list":
			gitArgs = []string{"stash", "list"}
		case "apply":
			gitArgs = []string{"stash", "apply"}
			if args.Index > 0 {
				gitArgs = append(gitArgs, fmt.Sprintf("stash@{%d}", args.Index))
			}
		case "drop":
			gitArgs = []string{"stash", "drop"}
			if args.Index > 0 {
				gitArgs = append(gitArgs, fmt.Sprintf("stash@{%d}", args.Index))
			}
		default:
			return nil, fmt.Errorf("git_stash: unknown action %q, use one of: save, pop, list, apply, drop", args.Action)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_stash: %w", err)
		}
		return resultToMap(res), nil
	})
}
