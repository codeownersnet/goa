package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type cloneArgs struct {
	URL       string `json:"url"`
	Directory string `json:"directory"`
	Branch    string `json:"branch,omitempty"`
	Depth     int    `json:"depth,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
}

func newCloneTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_clone",
		Description: "Clones a repository into a new directory. Use branch to clone a specific branch, depth for shallow clone, recursive for submodules.",
	}, func(ctx context.Context, args cloneArgs) (map[string]any, error) {
		if err := guard.Check(args.Directory); err != nil {
			return nil, fmt.Errorf("git_clone: %w", err)
		}

		gitArgs := []string{"clone"}
		if args.Branch != "" {
			gitArgs = append(gitArgs, "-b", args.Branch)
		}
		if args.Depth > 0 {
			gitArgs = append(gitArgs, "--depth", fmt.Sprint(args.Depth))
		}
		if args.Recursive {
			gitArgs = append(gitArgs, "--recursive")
		}
		gitArgs = append(gitArgs, args.URL, args.Directory)

		res, err := runGit(ctx, cfg, guard, "", gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_clone: %w", err)
		}

		result := resultToMap(res)
		result["directory"] = args.Directory
		return result, nil
	})
}
