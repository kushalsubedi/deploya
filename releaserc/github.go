package releaserc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const githubAPI = "https://api.github.com"

// GitHub is a minimal GitHub API client.
type GitHub struct {
	token  string
	repo   string // owner/repo
	client *http.Client
}

// NewGitHub creates a new GitHub client.
func NewGitHub(token, repo string) *GitHub {
	return &GitHub{
		token:  token,
		repo:   repo,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// EnrichCommits fetches PR data for each commit and deduplicates by PR number.
func (g *GitHub) EnrichCommits(commits []CommitInfo) ([]CommitInfo, error) {
	seen := map[int]bool{}
	var enriched []CommitInfo

	for _, c := range commits {
		pr, err := g.commitPR(c.SHA)
		if err != nil || pr == nil {
			// No PR found — keep commit as-is
			enriched = append(enriched, c)
			continue
		}

		// Deduplicate by PR number
		if seen[pr.Number] {
			continue
		}
		seen[pr.Number] = true

		c.PRNumber = pr.Number
		c.Title = pr.Title
		c.Author = pr.User.Login
		if len(pr.Labels) > 0 {
			c.Label = pr.Labels[0].Name
		}
		enriched = append(enriched, c)
	}

	return enriched, nil
}

// CreateRelease creates a GitHub release and returns the release ID.
func (g *GitHub) CreateRelease(version, tag, body string) (int64, error) {
	payload := map[string]interface{}{
		"tag_name":         tag,
		"target_commitish": "main",
		"name":             version,
		"body":             body,
		"draft":            false,
		"prerelease":       false,
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/repos/%s/releases", githubAPI, g.repo)

	resp, err := g.post(url, data)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result.ID, nil
}

// UploadAsset uploads a file as a release asset.
func (g *GitHub) UploadAsset(releaseID int64, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("could not open file %s: %w", filePath, err)
	}
	defer f.Close()

	fileName := filepath.Base(filePath)
	ext := filepath.Ext(fileName)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	url := fmt.Sprintf(
		"https://uploads.github.com/repos/%s/releases/%d/assets?name=%s",
		g.repo, releaseID, fileName,
	)

	req, err := http.NewRequest(http.MethodPost, url, f)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", mimeType)

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed %d: %s", resp.StatusCode, string(b))
	}

	return nil
}

// ── internal ────────────────────────────────────────────────────────────────

type prResponse struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (g *GitHub) commitPR(sha string) (*prResponse, error) {
	url := fmt.Sprintf("%s/repos/%s/commits/%s/pulls", githubAPI, g.repo, sha)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var prs []prResponse
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, err
	}

	if len(prs) == 0 {
		return nil, nil
	}

	return &prs[0], nil
}

func (g *GitHub) post(url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	return g.client.Do(req)
}

// splitRepo splits "owner/repo" into owner and repo.
func splitRepo(repo string) (string, string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", repo
	}
	return parts[0], parts[1]
}
