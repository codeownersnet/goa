package registry

import (
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/bash"
	"github.com/codeownersnet/goa/tool/difftool"
	"github.com/codeownersnet/goa/tool/editfile"
	"github.com/codeownersnet/goa/tool/exitlooptool"
	"github.com/codeownersnet/goa/tool/git"
	"github.com/codeownersnet/goa/tool/github"
	"github.com/codeownersnet/goa/tool/glob"
	"github.com/codeownersnet/goa/tool/grep"
	"github.com/codeownersnet/goa/tool/listdir"
	"github.com/codeownersnet/goa/tool/readfile"
	"github.com/codeownersnet/goa/tool/writefile"
)

type builtinConfig struct {
	allowedPaths []string
	githubToken  string
}

type BuiltinOption func(*builtinConfig)

func WithBuiltinAllowedPaths(paths []string) BuiltinOption {
	return func(c *builtinConfig) { c.allowedPaths = paths }
}

func WithBuiltinGitHubToken(token string) BuiltinOption {
	return func(c *builtinConfig) { c.githubToken = token }
}

func DefaultBuiltinRegistry(opts ...BuiltinOption) *Registry {
	cfg := builtinConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return New(
		WithFactory("bash", func() (tool.Tool, error) {
			return bash.New(bash.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("read_file", func() (tool.Tool, error) {
			return readfile.New(readfile.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("write_file", func() (tool.Tool, error) {
			return writefile.New(writefile.Config{AllowedPaths: cfg.allowedPaths, CreateDirs: true})
		}),
		WithFactory("edit_file", func() (tool.Tool, error) {
			return editfile.New(editfile.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("glob", func() (tool.Tool, error) {
			return glob.New(glob.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("grep", func() (tool.Tool, error) {
			return grep.New(grep.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("list_dir", func() (tool.Tool, error) {
			return listdir.New(listdir.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("exit_loop", func() (tool.Tool, error) {
			return exitlooptool.New()
		}),
		WithFactory("diff", func() (tool.Tool, error) {
			return difftool.New(difftool.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_clone", func() (tool.Tool, error) {
			return git.NewCloneTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_pull", func() (tool.Tool, error) {
			return git.NewPullTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_push", func() (tool.Tool, error) {
			return git.NewPushTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_add", func() (tool.Tool, error) {
			return git.NewAddTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_branch", func() (tool.Tool, error) {
			return git.NewBranchTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_commit", func() (tool.Tool, error) {
			return git.NewCommitTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_stash", func() (tool.Tool, error) {
			return git.NewStashTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_status", func() (tool.Tool, error) {
			return git.NewStatusTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_diff", func() (tool.Tool, error) {
			return git.NewDiffTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_log", func() (tool.Tool, error) {
			return git.NewLogTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("git_checkout", func() (tool.Tool, error) {
			return git.NewCheckoutTool(git.Config{AllowedPaths: cfg.allowedPaths})
		}),
		WithFactory("github_get_pull_request", func() (tool.Tool, error) {
			return github.NewGetPullRequestTool(github.Config{Token: cfg.githubToken})
		}),
		WithFactory("github_list_pr_files", func() (tool.Tool, error) {
			return github.NewListPRFilesTool(github.Config{Token: cfg.githubToken})
		}),
		WithFactory("github_list_review_comments", func() (tool.Tool, error) {
			return github.NewListReviewCommentsTool(github.Config{Token: cfg.githubToken})
		}),
		WithFactory("github_create_review", func() (tool.Tool, error) {
			return github.NewCreateReviewTool(github.Config{Token: cfg.githubToken})
		}),
		WithFactory("github_add_pr_comment", func() (tool.Tool, error) {
			return github.NewAddPRCommentTool(github.Config{Token: cfg.githubToken})
		}),
		WithFactory("github_add_review_comment", func() (tool.Tool, error) {
			return github.NewAddReviewCommentTool(github.Config{Token: cfg.githubToken})
		}),
		WithFactory("github_reply_to_review_comment", func() (tool.Tool, error) {
			return github.NewReplyToReviewCommentTool(github.Config{Token: cfg.githubToken})
		}),
	)
}
