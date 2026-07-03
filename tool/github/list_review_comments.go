package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v88/github"
)

type listReviewCommentsArgs struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	PullNumber int    `json:"pull_number"`
	PerPage    int    `json:"per_page,omitempty"`
	Page       int    `json:"page,omitempty"`
}

type reviewCommentItem struct {
	ID               int64  `json:"id"`
	InReplyToID      int64  `json:"in_reply_to_id,omitempty"`
	Path             string `json:"path"`
	Body             string `json:"body"`
	Line             int    `json:"line,omitempty"`
	OriginalLine     int    `json:"original_line,omitempty"`
	Side             string `json:"side,omitempty"`
	SubjectType      string `json:"subject_type,omitempty"`
	CommitID         string `json:"commit_id"`
	OriginalCommitID string `json:"original_commit_id"`
	HTMLURL          string `json:"html_url"`
	User             string `json:"user"`
}

type reviewCommentsResult struct {
	Comments []reviewCommentItem `json:"comments"`
	HasMore  bool                `json:"has_more"`
}

func listReviewComments(ctx context.Context, client *github.Client, args listReviewCommentsArgs) (reviewCommentsResult, error) {
	opts := &github.PullRequestListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: args.PerPage,
			Page:    args.Page,
		},
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 100
	}

	comments, resp, err := client.PullRequests.ListComments(ctx, args.Owner, args.Repo, args.PullNumber, opts)
	if err != nil {
		return reviewCommentsResult{}, fmt.Errorf("github_list_review_comments: %w", err)
	}
	defer resp.Body.Close()

	result := reviewCommentsResult{
		Comments: make([]reviewCommentItem, 0, len(comments)),
		HasMore:  resp.NextPage > 0,
	}

	for _, c := range comments {
		user := ""
		if c.User != nil {
			user = c.User.GetLogin()
		}

		result.Comments = append(result.Comments, reviewCommentItem{
			ID:               c.GetID(),
			InReplyToID:      c.GetInReplyTo(),
			Path:             c.GetPath(),
			Body:             c.GetBody(),
			Line:             c.GetLine(),
			OriginalLine:     c.GetOriginalLine(),
			Side:             c.GetSide(),
			SubjectType:      c.GetSubjectType(),
			CommitID:         c.GetCommitID(),
			OriginalCommitID: c.GetOriginalCommitID(),
			HTMLURL:          c.GetHTMLURL(),
			User:             user,
		})
	}

	return result, nil
}
