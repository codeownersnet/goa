package git

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type diffArgs struct {
	WorkDir string   `json:"workdir,omitempty"`
	Target  string   `json:"target,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	Stat    bool     `json:"stat,omitempty"`
	Context int      `json:"context,omitempty"`
}

func newDiffTool(cfg Config, guard *pathguard.PathGuard) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "git_diff",
		Description: "Shows changes between commits, commit and working tree, etc. Target controls what to diff: staged (cached changes vs HEAD), unstaged (working tree vs index), or a git ref (e.g. HEAD~3, main). Stat shows summary only. Context sets number of context lines.",
	}, func(ctx context.Context, args diffArgs) (map[string]any, error) {
		workDir := resolveWorkDir(args.WorkDir, cfg)

		var gitArgs []string
		switch args.Target {
		case "staged":
			gitArgs = []string{"diff", "--cached"}
		case "unstaged", "":
			gitArgs = []string{"diff"}
		default:
			gitArgs = []string{"diff", args.Target}
		}

		if args.Stat {
			gitArgs = append(gitArgs, "--stat")
		}
		if args.Context > 0 {
			gitArgs = append(gitArgs, fmt.Sprintf("--unified=%d", args.Context))
		}
		if len(args.Paths) > 0 {
			gitArgs = append(gitArgs, "--")
			gitArgs = append(gitArgs, args.Paths...)
		}

		res, err := runGit(ctx, cfg, guard, workDir, gitArgs...)
		if err != nil {
			return nil, fmt.Errorf("git_diff: %w", err)
		}
		return resultToMap(res), nil
	})
}
