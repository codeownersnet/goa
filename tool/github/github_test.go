package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-github/v88/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testClient(t *testing.T, server *httptest.Server) *github.Client {
	t.Helper()
	baseURL := server.URL + "/"
	cr, err := newClientWithOptions(Config{Token: "test-token", Timeout: 10}, github.WithURLs(&baseURL, nil))
	require.NoError(t, err)
	require.NoError(t, cr.tokenErr)
	return cr.client
}

func TestGetPullRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/42", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":   42,
			"state":    "open",
			"title":    "Test PR",
			"body":     "PR body",
			"draft":    false,
			"html_url": "https://github.com/owner/repo/pull/42",
			"head": map[string]any{
				"sha": "abc123",
			},
			"base": map[string]any{
				"sha": "def456",
			},
			"user": map[string]any{
				"login": "reviewer",
			},
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	result, err := getPullRequest(context.Background(), client, getPullRequestArgs{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 42,
	})
	require.NoError(t, err)
	assert.Equal(t, 42, result.Number)
	assert.Equal(t, "open", result.State)
	assert.Equal(t, "Test PR", result.Title)
	assert.Equal(t, "abc123", result.HeadSHA)
	assert.Equal(t, "def456", result.BaseSHA)
	assert.Equal(t, "reviewer", result.User)
}

func TestListPRFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/42/files", r.URL.Path)
		w.Header().Set("Link", "")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"filename":  "main.go",
				"status":    "modified",
				"additions": 5,
				"deletions": 1,
				"changes":   6,
				"patch":     "@@ -1,5 +1,5 @@",
				"html_url":  "https://github.com/owner/repo/blob/main.go",
			},
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	result, err := listPRFiles(context.Background(), client, listPRFilesArgs{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 42,
	})
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	assert.Equal(t, "main.go", result.Files[0].Filename)
	assert.Equal(t, "modified", result.Files[0].Status)
	assert.Equal(t, 6, result.Files[0].Changes)
	assert.False(t, result.HasMore)
}

func TestListReviewComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/42/comments", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":                 123,
				"in_reply_to_id":     0,
				"path":               "main.go",
				"body":               "Consider renaming.",
				"line":               10,
				"side":               "RIGHT",
				"subject_type":       "line",
				"commit_id":          "abc123",
				"original_commit_id": "abc123",
				"html_url":           "https://github.com/owner/repo/pull/42#discussion_r123",
				"user": map[string]any{
					"login": "reviewer",
				},
			},
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	result, err := listReviewComments(context.Background(), client, listReviewCommentsArgs{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 42,
	})
	require.NoError(t, err)
	require.Len(t, result.Comments, 1)
	assert.Equal(t, int64(123), result.Comments[0].ID)
	assert.Equal(t, "main.go", result.Comments[0].Path)
	assert.Equal(t, "reviewer", result.Comments[0].User)
}

func TestCreateReview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/42/reviews", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           999,
			"state":        "REQUEST_CHANGES",
			"body":         "Please address the comments.",
			"html_url":     "https://github.com/owner/repo/pull/42#pullrequestreview-999",
			"submitted_at": time.Now().UTC().Format(time.RFC3339),
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	result, err := createReview(context.Background(), client, createReviewArgs{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 42,
		CommitID:   "abc123",
		Body:       "Please address the comments.",
		Event:      "REQUEST_CHANGES",
		Comments: []reviewCommentInput{
			{
				Path: "main.go",
				Body: "Rename this variable.",
				Line: 10,
				Side: "RIGHT",
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(999), result.ID)
	assert.Equal(t, "REQUEST_CHANGES", result.State)
}

func TestAddPRComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/42/comments", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         777,
			"body":       "Top-level comment.",
			"html_url":   "https://github.com/owner/repo/issues/42#issuecomment-777",
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"updated_at": time.Now().UTC().Format(time.RFC3339),
			"user": map[string]any{
				"login": "reviewer",
			},
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	result, err := addPRComment(context.Background(), client, addPRCommentArgs{
		Owner:       "owner",
		Repo:        "repo",
		IssueNumber: 42,
		Body:        "Top-level comment.",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(777), result.ID)
	assert.Equal(t, "Top-level comment.", result.Body)
	assert.Equal(t, "reviewer", result.User)
}

func TestAddReviewComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/42/comments", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           888,
			"path":         "main.go",
			"body":         "Line comment.",
			"line":         10,
			"side":         "RIGHT",
			"subject_type": "line",
			"commit_id":    "abc123",
			"html_url":     "https://github.com/owner/repo/pull/42#discussion_r888",
			"user": map[string]any{
				"login": "reviewer",
			},
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	result, err := addReviewComment(context.Background(), client, addReviewCommentArgs{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 42,
		CommitID:   "abc123",
		Path:       "main.go",
		Body:       "Line comment.",
		Line:       10,
		Side:       "RIGHT",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(888), result.ID)
	assert.Equal(t, "main.go", result.Path)
}

func TestReplyToReviewComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/42/comments", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             444,
			"in_reply_to_id": 123,
			"path":           "main.go",
			"body":           "Good point, fixed.",
			"commit_id":      "abc123",
			"html_url":       "https://github.com/owner/repo/pull/42#discussion_r444",
			"user": map[string]any{
				"login": "author",
			},
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	result, err := replyToReviewComment(context.Background(), client, replyToReviewCommentArgs{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 42,
		CommentID:  123,
		Body:       "Good point, fixed.",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(444), result.ID)
	assert.Equal(t, int64(123), result.InReplyToID)
	assert.Equal(t, "author", result.User)
}

func TestResolveToken(t *testing.T) {
	t.Run("explicit token", func(t *testing.T) {
		token, err := resolveToken(Config{Token: "explicit"})
		require.NoError(t, err)
		assert.Equal(t, "explicit", token)
	})

	t.Run("env token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "env-token")
		token, err := resolveToken(Config{})
		require.NoError(t, err)
		assert.Equal(t, "env-token", token)
	})

	t.Run("missing token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		_, err := resolveToken(Config{})
		assert.Error(t, err)
	})
}

func TestAllTools(t *testing.T) {
	ts, err := AllTools(Config{Token: "test-token"})
	require.NoError(t, err)
	require.Len(t, ts.Tools(), 7)

	names := make(map[string]bool)
	for _, t := range ts.Tools() {
		names[t.Name()] = true
	}
	expected := []string{
		"github_get_pull_request",
		"github_list_pr_files",
		"github_list_review_comments",
		"github_create_review",
		"github_add_pr_comment",
		"github_add_review_comment",
		"github_reply_to_review_comment",
	}
	for _, name := range expected {
		assert.True(t, names[name], "missing tool %s", name)
	}
}

func TestToolsConstructWithoutTokenButFailOnProcess(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	getTool, err := NewGetPullRequestTool(Config{})
	require.NoError(t, err)

	_, err = getTool.Process(context.Background(), map[string]any{
		"owner":       "owner",
		"repo":        "repo",
		"pull_number": 1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GITHUB_TOKEN")
}

func TestErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Not Found",
		})
	}))
	defer server.Close()

	client := testClient(t, server)
	_, err := getPullRequest(context.Background(), client, getPullRequestArgs{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 999,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github_get_pull_request")
}

func ExampleNewGetPullRequestTool() {
	tool, err := NewGetPullRequestTool(Config{Token: "example-token"})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(tool.Name())
	// Output: github_get_pull_request
}
