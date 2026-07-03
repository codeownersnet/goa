package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v88/github"
)

type listPRFilesArgs struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	PullNumber int    `json:"pull_number"`
	PerPage    int    `json:"per_page,omitempty"`
	Page       int    `json:"page,omitempty"`
}

type prFileResult struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch,omitempty"`
	BlobURL   string `json:"blob_url"`
}

type prFilesResult struct {
	Files   []prFileResult `json:"files"`
	HasMore bool           `json:"has_more"`
}

func listPRFiles(ctx context.Context, client *github.Client, args listPRFilesArgs) (prFilesResult, error) {
	opts := &github.ListOptions{
		PerPage: args.PerPage,
		Page:    args.Page,
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 100
	}

	files, resp, err := client.PullRequests.ListFiles(ctx, args.Owner, args.Repo, args.PullNumber, opts)
	if err != nil {
		return prFilesResult{}, fmt.Errorf("github_list_pr_files: %w", err)
	}
	defer resp.Body.Close()

	result := prFilesResult{
		Files:   make([]prFileResult, 0, len(files)),
		HasMore: resp.NextPage > 0,
	}

	for _, f := range files {
		result.Files = append(result.Files, prFileResult{
			Filename:  f.GetFilename(),
			Status:    f.GetStatus(),
			Additions: f.GetAdditions(),
			Deletions: f.GetDeletions(),
			Changes:   f.GetChanges(),
			Patch:     f.GetPatch(),
			BlobURL:   f.GetBlobURL(),
		})
	}

	return result, nil
}
