package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v88/github"
)

type reviewCommentInput struct {
	Path        string `json:"path"`
	Body        string `json:"body"`
	Line        int    `json:"line,omitempty"`
	Side        string `json:"side,omitempty"`
	StartLine   int    `json:"start_line,omitempty"`
	StartSide   string `json:"start_side,omitempty"`
	SubjectType string `json:"subject_type,omitempty"`
}

type createReviewArgs struct {
	Owner      string               `json:"owner"`
	Repo       string               `json:"repo"`
	PullNumber int                  `json:"pull_number"`
	CommitID   string               `json:"commit_id,omitempty"`
	Body       string               `json:"body,omitempty"`
	Event      string               `json:"event,omitempty"`
	Comments   []reviewCommentInput `json:"comments,omitempty"`
}

type reviewResult struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	Submitted string `json:"submitted_at"`
}

func createReview(ctx context.Context, client *github.Client, args createReviewArgs) (reviewResult, error) {
	comments := make([]*github.DraftReviewComment, 0, len(args.Comments))
	for _, c := range args.Comments {
		drc := &github.DraftReviewComment{
			Path:      stringPtrIfNonEmpty(c.Path),
			Body:      stringPtrIfNonEmpty(c.Body),
			Line:      intPtrIfPositive(c.Line),
			Side:      stringPtrIfNonEmpty(c.Side),
			StartLine: intPtrIfPositive(c.StartLine),
			StartSide: stringPtrIfNonEmpty(c.StartSide),
		}
		comments = append(comments, drc)
	}

	review := &github.PullRequestReviewRequest{
		CommitID: stringPtrIfNonEmpty(args.CommitID),
		Body:     stringPtrIfNonEmpty(args.Body),
		Event:    stringPtrIfNonEmpty(args.Event),
		Comments: comments,
	}

	created, resp, err := client.PullRequests.CreateReview(ctx, args.Owner, args.Repo, args.PullNumber, review)
	if err != nil {
		return reviewResult{}, fmt.Errorf("github_create_review: %w", err)
	}
	defer resp.Body.Close()

	return reviewResult{
		ID:        created.GetID(),
		State:     created.GetState(),
		Body:      created.GetBody(),
		HTMLURL:   created.GetHTMLURL(),
		Submitted: created.GetSubmittedAt().String(),
	}, nil
}
