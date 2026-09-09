package pr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultGeminiModel  = "gemini-2.0-flash"
	fallbackGeminiModel = "gemini-1.5-flash"
	geminiBaseURL       = "https://generativelanguage.googleapis.com/v1beta/models"
)

// GeminiClient interacts with Google Gemini's REST API.
type GeminiClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewGeminiClient initializes a client with the given API key.
// If key is empty, it checks GEMINI_API_KEY and GOOGLE_API_KEY environment variables.
func NewGeminiClient(key string) *GeminiClient {
	if key == "" {
		key = os.Getenv("GEMINI_API_KEY")
	}
	if key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
	}

	return &GeminiClient{
		apiKey:     key,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// IsAvailable returns true if an API key is present.
func (c *GeminiClient) IsAvailable() bool {
	return strings.TrimSpace(c.apiKey) != ""
}

type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature      float64 `json:"temperature"`
	MaxOutputTokens  int     `json:"maxOutputTokens"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// GeneratePR sends repository context, commits, and diff to Gemini AI
// and returns a structured, beautiful Pull Request title and description.
func (c *GeminiClient) GeneratePR(ctx context.Context, repo, head, base string, commits []Commit, diffStat, diffPatch string) (*PRContent, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("gemini API key is not configured")
	}

	prompt := buildPrompt(repo, head, base, commits, diffStat, diffPatch)

	// Try default model first, fallback to gemini-1.5-flash if needed
	content, err := c.callModel(ctx, defaultGeminiModel, prompt)
	if err != nil && !strings.Contains(err.Error(), "API_KEY_INVALID") {
		// Attempt fallback model
		fallbackContent, fallbackErr := c.callModel(ctx, fallbackGeminiModel, prompt)
		if fallbackErr == nil {
			return fallbackContent, nil
		}
	}

	return content, err
}

func (c *GeminiClient) callModel(ctx context.Context, model, prompt string) (*PRContent, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			Temperature:      0.2,
			MaxOutputTokens:  2500,
			ResponseMimeType: "application/json",
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s:generateContent", geminiBaseURL, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gemini response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var gErr geminiResponse
		if json.Unmarshal(respBytes, &gErr) == nil && gErr.Error != nil {
			return nil, fmt.Errorf("gemini API error (%d): %s", gErr.Error.Code, gErr.Error.Message)
		}
		return nil, fmt.Errorf("gemini API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var gResp geminiResponse
	if err := json.Unmarshal(respBytes, &gResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no content candidates")
	}

	rawText := gResp.Candidates[0].Content.Parts[0].Text
	return ParseGeminiPRResponse(rawText)
}

func buildPrompt(repo, head, base string, commits []Commit, diffStat, diffPatch string) string {
	var commitList strings.Builder
	for _, c := range commits {
		authorStr := ""
		if c.Author != "" {
			authorStr = fmt.Sprintf(" by %s", c.Author)
		}
		commitList.WriteString(fmt.Sprintf("- %s: %s%s\n", c.Short, c.Title, authorStr))
	}

	return fmt.Sprintf(`You are an expert software engineer and technical communicator writing a GitHub Pull Request description.
Analyze the following git information and generate a professional, aesthetic, and comprehensive Pull Request.

Repository: %s
Source Branch (HEAD): %s
Target Branch (BASE): %s

Unmerged Commits:
%s

Files Changed (Diff Stat):
%s

Code Diff:
%s

Instructions:
1. Generate a concise, informative Pull Request title adhering to Conventional Commits (e.g. "feat(ci): add automated PR creation with AI").
2. Generate a beautifully structured Markdown description containing:
   - ## 🎯 Overview: What this PR introduces, why it was made, and the problem it solves.
   - ## ✨ Key Changes: Highlight major features, bug fixes, or improvements in bullet points.
   - ## 🔍 Architectural & Implementation Notes: Notable changes to logic, workflows, or configurations.
   - ## 🧪 Testing & Verification: How these changes were verified or how reviewers can test them.
   - ## 📋 PR Checklist: Standard checklist (tested locally, followed conventions, etc.).
   - At the bottom: "<sub>Generated with [Deploya](https://github.com/kushalsubedi/deploya) and Gemini AI</sub>"

Output strictly valid JSON with this exact schema:
{
  "title": "...",
  "body": "..."
}
`, repo, head, base, commitList.String(), diffStat, diffPatch)
}

// ParseGeminiPRResponse extracts PRContent from raw text returned by the model.
func ParseGeminiPRResponse(raw string) (*PRContent, error) {
	raw = strings.TrimSpace(raw)

	// Strip markdown code fence if present
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	} else if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	}

	var content PRContent
	if err := json.Unmarshal([]byte(raw), &content); err == nil && content.Title != "" && content.Body != "" {
		return &content, nil
	}

	// Fallback parsing: if model returned plain markdown
	lines := strings.Split(raw, "\n")
	title := ""
	var bodyLines []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if title == "" && trimmed != "" {
			title = strings.TrimPrefix(trimmed, "# ")
			title = strings.TrimPrefix(title, "Title: ")
			title = strings.Trim(title, `"'*`)
			continue
		}
		if title != "" {
			bodyLines = append(bodyLines, lines[i])
		}
	}

	if title == "" {
		title = "Pull Request"
	}

	body := strings.TrimSpace(strings.Join(bodyLines, "\n"))
	if body == "" {
		body = raw
	}

	return &PRContent{
		Title: title,
		Body:  body,
	}, nil
}
