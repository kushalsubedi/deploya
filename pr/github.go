package pr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const githubAPIBase = "https://api.github.com"

// GitHubClient handles interaction with GitHub Pull Requests API.
type GitHubClient struct {
	token      string
	httpClient *http.Client
}

// NewGitHubClient creates a GitHub client with the given token.
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		token:      token,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// ResolveGitHubToken attempts to read a GitHub token from:
// 1. GH_TOKEN environment variable
// 2. GITHUB_TOKEN environment variable
// 3. GitHub CLI ('gh auth token')
func ResolveGitHubToken() (string, error) {
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token, nil
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}

	// Try reading token from gh CLI if available
	if path, err := exec.LookPath("gh"); err == nil && path != "" {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return strings.TrimSpace(string(out)), nil
		}
	}

	return "", fmt.Errorf("no GitHub token found\nSet GH_TOKEN (or GITHUB_TOKEN) with: export GH_TOKEN=your_token\nor log in with the GitHub CLI: gh auth login")
}

// CreatePullRequest submits a new pull request to GitHub.
func (g *GitHubClient) CreatePullRequest(repo, title, head, base, body string, draft bool) (*PullRequestResult, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls", githubAPIBase, repo)

	payload := map[string]interface{}{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
		"draft": draft,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	g.setHeaders(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusCreated {
		var prResult PullRequestResult
		if err := json.Unmarshal(respBody, &prResult); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &prResult, nil
	}

	// If PR already exists, locate it
	if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(string(respBody), "A pull request already exists") {
		existing, findErr := g.FindExistingPR(repo, head, base)
		if findErr == nil && existing != nil {
			return existing, fmt.Errorf("a pull request already exists for this branch: %s", existing.HTMLURL)
		}
	}

	return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(respBody))
}

// FindExistingPR searches for an open PR for the given head and base.
func (g *GitHubClient) FindExistingPR(repo, head, base string) (*PullRequestResult, error) {
	// Query open pull requests for head branch
	url := fmt.Sprintf("%s/repos/%s/pulls?state=open&base=%s", githubAPIBase, repo, base)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	g.setHeaders(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list PRs: HTTP %d", resp.StatusCode)
	}

	var prs []struct {
		ID      int64  `json:"id"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
		Draft   bool   `json:"draft"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, err
	}

	for _, p := range prs {
		if p.Head.Ref == head {
			return &PullRequestResult{
				ID:      p.ID,
				Number:  p.Number,
				Title:   p.Title,
				HTMLURL: p.HTMLURL,
				State:   p.State,
				Draft:   p.Draft,
				Head:    p.Head.Ref,
				Base:    p.Base.Ref,
			}, nil
		}
	}

	return nil, nil
}

func (g *GitHubClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
}
