package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v88/github"
)

type addPRCommentArgs struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	IssueNumber int    `json:"issue_number"`
	Body        string `json:"body"`
}

type issueCommentResult struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	User      string `json:"user"`
}

func addPRComment(ctx context.Context, client *github.Client, args addPRCommentArgs) (issueCommentResult, error) {
	body := args.Body
	comment := &github.IssueComment{Body: &body}
	created, resp, err := client.Issues.CreateComment(ctx, args.Owner, args.Repo, args.IssueNumber, comment)
	if err != nil {
		return issueCommentResult{}, fmt.Errorf("github_add_pr_comment: %w", err)
	}
	defer resp.Body.Close()

	user := ""
	if created.User != nil {
		user = created.User.GetLogin()
	}

	return issueCommentResult{
		ID:        created.GetID(),
		Body:      created.GetBody(),
		HTMLURL:   created.GetHTMLURL(),
		CreatedAt: created.GetCreatedAt().String(),
		UpdatedAt: created.GetUpdatedAt().String(),
		User:      user,
	}, nil
}
