package github

import (
	"context"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type Toolset struct {
	tools []tool.Tool
}

func (ts *Toolset) Tools() []tool.Tool { return ts.tools }

var _ tool.Toolset = (*Toolset)(nil)

func AllTools(cfg Config) (*Toolset, error) {
	tools := make([]tool.Tool, 0, 7)

	creators := []func(Config) (tool.Tool, error){
		NewGetPullRequestTool,
		NewListPRFilesTool,
		NewListReviewCommentsTool,
		NewCreateReviewTool,
		NewAddPRCommentTool,
		NewAddReviewCommentTool,
		NewReplyToReviewCommentTool,
	}

	for _, creator := range creators {
		t, err := creator(cfg)
		if err != nil {
			return nil, fmt.Errorf("github AllTools: %w", err)
		}
		tools = append(tools, t)
	}

	return &Toolset{tools: tools}, nil
}

func NewGetPullRequestTool(cfg Config) (tool.Tool, error) {
	cr, err := newClientWithOptions(cfg)
	if err != nil {
		return nil, err
	}
	return functiontool.New(functiontool.Config{
		Name:        "github_get_pull_request",
		Description: "Get metadata for a GitHub pull request including title, body, head/base commit SHAs, state, and URL.",
	}, func(ctx context.Context, args getPullRequestArgs) (pullRequestResult, error) {
		if cr.tokenErr != nil {
			return pullRequestResult{}, cr.tokenErr
		}
		return getPullRequest(ctx, cr.client, args)
	})
}

func NewListPRFilesTool(cfg Config) (tool.Tool, error) {
	cr, err := newClientWithOptions(cfg)
	if err != nil {
		return nil, err
	}
	return functiontool.New(functiontool.Config{
		Name:        "github_list_pr_files",
		Description: "List files changed in a GitHub pull request with status, additions, deletions, and patch snippets.",
	}, func(ctx context.Context, args listPRFilesArgs) (prFilesResult, error) {
		if cr.tokenErr != nil {
			return prFilesResult{}, cr.tokenErr
		}
		return listPRFiles(ctx, cr.client, args)
	})
}

func NewListReviewCommentsTool(cfg Config) (tool.Tool, error) {
	cr, err := newClientWithOptions(cfg)
	if err != nil {
		return nil, err
	}
	return functiontool.New(functiontool.Config{
		Name:        "github_list_review_comments",
		Description: "List review comments on a GitHub pull request for threading and replies.",
	}, func(ctx context.Context, args listReviewCommentsArgs) (reviewCommentsResult, error) {
		if cr.tokenErr != nil {
			return reviewCommentsResult{}, cr.tokenErr
		}
		return listReviewComments(ctx, cr.client, args)
	})
}

func NewCreateReviewTool(cfg Config) (tool.Tool, error) {
	cr, err := newClientWithOptions(cfg)
	if err != nil {
		return nil, err
	}
	return functiontool.New(functiontool.Config{
		Name:        "github_create_review",
		Description: "Create a GitHub pull request review (APPROVE, REQUEST_CHANGES, or COMMENT) with optional line/file comments.",
	}, func(ctx context.Context, args createReviewArgs) (reviewResult, error) {
		if cr.tokenErr != nil {
			return reviewResult{}, cr.tokenErr
		}
		return createReview(ctx, cr.client, args)
	})
}

func NewAddPRCommentTool(cfg Config) (tool.Tool, error) {
	cr, err := newClientWithOptions(cfg)
	if err != nil {
		return nil, err
	}
	return functiontool.New(functiontool.Config{
		Name:        "github_add_pr_comment",
		Description: "Post a top-level comment on a GitHub pull request (issue comment).",
	}, func(ctx context.Context, args addPRCommentArgs) (issueCommentResult, error) {
		if cr.tokenErr != nil {
			return issueCommentResult{}, cr.tokenErr
		}
		return addPRComment(ctx, cr.client, args)
	})
}

func NewAddReviewCommentTool(cfg Config) (tool.Tool, error) {
	cr, err := newClientWithOptions(cfg)
	if err != nil {
		return nil, err
	}
	return functiontool.New(functiontool.Config{
		Name:        "github_add_review_comment",
		Description: "Add a single line-level or file-level review comment to a GitHub pull request.",
	}, func(ctx context.Context, args addReviewCommentArgs) (reviewCommentResult, error) {
		if cr.tokenErr != nil {
			return reviewCommentResult{}, cr.tokenErr
		}
		return addReviewComment(ctx, cr.client, args)
	})
}

func NewReplyToReviewCommentTool(cfg Config) (tool.Tool, error) {
	cr, err := newClientWithOptions(cfg)
	if err != nil {
		return nil, err
	}
	return functiontool.New(functiontool.Config{
		Name:        "github_reply_to_review_comment",
		Description: "Reply to an existing review comment thread on a GitHub pull request.",
	}, func(ctx context.Context, args replyToReviewCommentArgs) (reviewCommentResult, error) {
		if cr.tokenErr != nil {
			return reviewCommentResult{}, cr.tokenErr
		}
		return replyToReviewComment(ctx, cr.client, args)
	})
}
