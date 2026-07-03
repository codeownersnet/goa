package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v88/github"
)

type getPullRequestArgs struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	PullNumber int    `json:"pull_number"`
}

type pullRequestResult struct {
	Number  int    `json:"number"`
	State   string `json:"state"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HeadSHA string `json:"head_sha"`
	BaseSHA string `json:"base_sha"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
	User    string `json:"user"`
}

func getPullRequest(ctx context.Context, client *github.Client, args getPullRequestArgs) (pullRequestResult, error) {
	pr, resp, err := client.PullRequests.Get(ctx, args.Owner, args.Repo, args.PullNumber)
	if err != nil {
		return pullRequestResult{}, fmt.Errorf("github_get_pull_request: %w", err)
	}
	defer resp.Body.Close()

	user := ""
	if pr.User != nil {
		user = pr.User.GetLogin()
	}

	headSHA := ""
	if pr.Head != nil {
		headSHA = pr.Head.GetSHA()
	}

	baseSHA := ""
	if pr.Base != nil {
		baseSHA = pr.Base.GetSHA()
	}

	return pullRequestResult{
		Number:  pr.GetNumber(),
		State:   pr.GetState(),
		Title:   pr.GetTitle(),
		Body:    pr.GetBody(),
		HeadSHA: headSHA,
		BaseSHA: baseSHA,
		HTMLURL: pr.GetHTMLURL(),
		Draft:   pr.GetDraft(),
		User:    user,
	}, nil
}
