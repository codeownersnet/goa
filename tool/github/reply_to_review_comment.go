package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v88/github"
)

type replyToReviewCommentArgs struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	PullNumber int    `json:"pull_number"`
	CommentID  int64  `json:"comment_id"`
	Body       string `json:"body"`
}

func replyToReviewComment(ctx context.Context, client *github.Client, args replyToReviewCommentArgs) (reviewCommentResult, error) {
	created, resp, err := client.PullRequests.CreateCommentInReplyTo(ctx, args.Owner, args.Repo, args.PullNumber, args.Body, args.CommentID)
	if err != nil {
		return reviewCommentResult{}, fmt.Errorf("github_reply_to_review_comment: %w", err)
	}
	defer resp.Body.Close()

	return reviewCommentResultFromGithub(created), nil
}
