package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v88/github"
)

type addReviewCommentArgs struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	PullNumber  int    `json:"pull_number"`
	CommitID    string `json:"commit_id"`
	Path        string `json:"path"`
	Body        string `json:"body"`
	Line        int    `json:"line,omitempty"`
	Side        string `json:"side,omitempty"`
	StartLine   int    `json:"start_line,omitempty"`
	StartSide   string `json:"start_side,omitempty"`
	SubjectType string `json:"subject_type,omitempty"`
}

type reviewCommentResult struct {
	ID          int64  `json:"id"`
	InReplyToID int64  `json:"in_reply_to_id,omitempty"`
	Path        string `json:"path"`
	Body        string `json:"body"`
	Line        int    `json:"line,omitempty"`
	Side        string `json:"side,omitempty"`
	SubjectType string `json:"subject_type,omitempty"`
	CommitID    string `json:"commit_id"`
	HTMLURL     string `json:"html_url"`
	User        string `json:"user"`
}

func addReviewComment(ctx context.Context, client *github.Client, args addReviewCommentArgs) (reviewCommentResult, error) {
	comment := &github.PullRequestComment{
		CommitID:    stringPtrIfNonEmpty(args.CommitID),
		Path:        stringPtrIfNonEmpty(args.Path),
		Body:        stringPtrIfNonEmpty(args.Body),
		Line:        intPtrIfPositive(args.Line),
		Side:        stringPtrIfNonEmpty(args.Side),
		StartLine:   intPtrIfPositive(args.StartLine),
		StartSide:   stringPtrIfNonEmpty(args.StartSide),
		SubjectType: stringPtrIfNonEmpty(args.SubjectType),
	}

	created, resp, err := client.PullRequests.CreateComment(ctx, args.Owner, args.Repo, args.PullNumber, comment)
	if err != nil {
		return reviewCommentResult{}, fmt.Errorf("github_add_review_comment: %w", err)
	}
	defer resp.Body.Close()

	return reviewCommentResultFromGithub(created), nil
}

func reviewCommentResultFromGithub(c *github.PullRequestComment) reviewCommentResult {
	user := ""
	if c.User != nil {
		user = c.User.GetLogin()
	}

	return reviewCommentResult{
		ID:          c.GetID(),
		InReplyToID: c.GetInReplyTo(),
		Path:        c.GetPath(),
		Body:        c.GetBody(),
		Line:        c.GetLine(),
		Side:        c.GetSide(),
		SubjectType: c.GetSubjectType(),
		CommitID:    c.GetCommitID(),
		HTMLURL:     c.GetHTMLURL(),
		User:        user,
	}
}
