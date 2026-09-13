package pr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
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
		apiKey:     strings.TrimSpace(key),
		httpClient: &http.Client{Timeout: 35 * time.Second},
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
// and returns a concise, structured, and beautiful Pull Request title and description.
func (c *GeminiClient) GeneratePR(ctx context.Context, repo, head, base string, commits []Commit, diffStat, diffPatch string) (*PRContent, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("gemini API key is not configured")
	}

	prompt := buildPrompt(repo, head, base, commits, diffPatch)

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
			MaxOutputTokens:  4096,
			ResponseMimeType: "application/json",
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Send key in query parameter as well as header for maximum proxy/gateway compatibility
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, model, c.apiKey)
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

	if len(gResp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini returned no content candidates")
	}

	var sb strings.Builder
	for _, part := range gResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}

	rawText := strings.TrimSpace(sb.String())
	if rawText == "" {
		return nil, fmt.Errorf("gemini candidate contained empty text")
	}

	return ParseGeminiPRResponse(rawText)
}

func buildPrompt(repo, head, base string, commits []Commit, diffPatch string) string {
	var commitList strings.Builder
	for _, c := range commits {
		authorStr := ""
		if c.Author != "" {
			authorStr = fmt.Sprintf(" by %s", c.Author)
		}
		commitList.WriteString(fmt.Sprintf("- %s: %s%s\n", c.Short, c.Title, authorStr))
	}

	// Limit diffPatch to prevent token exhaustion
	if len(diffPatch) > 25000 {
		diffPatch = diffPatch[:25000] + "\n\n[... diff truncated for length ...]"
	}

	return fmt.Sprintf(`You are an expert software engineer writing a clean, concise, and professional GitHub Pull Request.
Analyze the unmerged commits and code diff below, and synthesize a high-level summary.

Repository: %s
Source Branch (HEAD): %s
Target Branch (BASE): %s

Unmerged Commits:
%s

Code Diff:
%s

Instructions:
1. Title: A concise conventional commit title (e.g. "feat(pr): improve AI-generated summaries").
2. Body: Beautiful Markdown formatted as follows:
   - ## 🎯 Overview: Concise 2-3 sentence explanation of the objective, problem solved, and overall approach.
   - ## ✨ Key Changes: Grouped bullet points highlighting the main functional and architectural changes.
   - ## 🔍 Implementation Highlights: Bullet points explaining notable logic or design choices.
   - ## 🧪 Verification: Brief summary of tests run or how to verify.
   - ## 📋 Checklist: Standard review checklist items.
   - At the bottom: "<sub>Generated with [Deploya](https://github.com/kushalsubedi/deploya) and Gemini AI</sub>"

CRITICAL REQUIREMENTS:
- DO NOT list individual file names with line additions/deletions counts (+/-) like git diff/status.
- Keep the description concise, informative, and readable. Reviewers already have the 'Files changed' tab in GitHub.
- Return ONLY valid JSON with keys "title" and "body".

Output JSON format:
{
  "title": "...",
  "body": "..."
}
`, repo, head, base, commitList.String(), diffPatch)
}

// ParseGeminiPRResponse extracts PRContent from raw text returned by the model.
// It handles markdown fences, embedded JSON objects, unescaped newlines, and fallback markdown.
func ParseGeminiPRResponse(raw string) (*PRContent, error) {
	raw = strings.TrimSpace(raw)

	// 1. Extract content within markdown code fences if present
	if start := strings.Index(raw, "```json"); start != -1 {
		rest := raw[start+len("```json"):]
		if end := strings.Index(rest, "```"); end != -1 {
			raw = strings.TrimSpace(rest[:end])
		}
	} else if start := strings.Index(raw, "```"); start != -1 {
		rest := raw[start+len("```"):]
		if end := strings.Index(rest, "```"); end != -1 {
			raw = strings.TrimSpace(rest[:end])
		}
	}

	// 2. Direct JSON unmarshal
	var content PRContent
	if err := json.Unmarshal([]byte(raw), &content); err == nil && content.Title != "" && content.Body != "" {
		content.Title = cleanExtractedTitle(content.Title)
		return &content, nil
	}

	// 3. Extract outermost JSON object { ... } if text had surrounding conversational text
	if firstBrace := strings.Index(raw, "{"); firstBrace != -1 {
		if lastBrace := strings.LastIndex(raw, "}"); lastBrace != -1 && lastBrace > firstBrace {
			candidateJSON := raw[firstBrace : lastBrace+1]
			if err := json.Unmarshal([]byte(candidateJSON), &content); err == nil && content.Title != "" && content.Body != "" {
				content.Title = cleanExtractedTitle(content.Title)
				return &content, nil
			}

			// Try fixing unescaped literal control characters in JSON strings
			sanitized := fixUnescapedJSONNewlines(candidateJSON)
			if err := json.Unmarshal([]byte(sanitized), &content); err == nil && content.Title != "" && content.Body != "" {
				content.Title = cleanExtractedTitle(content.Title)
				return &content, nil
			}
		}
	}

	// 4. Regex extraction of "title" and "body"
	title, body := extractFieldsRegex(raw)
	if title != "" && body != "" {
		return &PRContent{
			Title: cleanExtractedTitle(title),
			Body:  body,
		}, nil
	}

	// 5. Fallback parsing: if model returned plain markdown without JSON
	lines := strings.Split(raw, "\n")
	title = ""
	var bodyLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if title == "" && trimmed != "" {
			candidate := strings.TrimPrefix(trimmed, "# ")
			candidate = strings.TrimPrefix(candidate, "Title: ")
			candidate = strings.Trim(candidate, `"'*`+"`")
			if candidate != "{" && !strings.HasPrefix(candidate, "\"title\"") {
				title = candidate
				continue
			}
		}
		if title != "" {
			bodyLines = append(bodyLines, line)
		}
	}

	if title == "" {
		title = "Pull Request"
	}

	body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	if body == "" {
		body = raw
	}

	return &PRContent{
		Title: cleanExtractedTitle(title),
		Body:  body,
	}, nil
}

func cleanExtractedTitle(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "Title: ")
	t = strings.Trim(t, `"'*`+"`")
	return strings.TrimSpace(t)
}

// extractFieldsRegex pulls out "title" and "body" values if standard JSON unmarshaling fails
func extractFieldsRegex(text string) (string, string) {
	titleRe := regexp.MustCompile(`"title"\s*:\s*"([^"\\]*(?:\\.[^"\\]*)*)"`)
	titleMatch := titleRe.FindStringSubmatch(text)
	var title string
	if len(titleMatch) > 1 {
		title = unescapeJSONString(titleMatch[1])
	}

	bodyRe := regexp.MustCompile(`"body"\s*:\s*"([^"\\]*(?:\\.[^"\\]*)*)"`)
	bodyMatch := bodyRe.FindStringSubmatch(text)
	var body string
	if len(bodyMatch) > 1 {
		body = unescapeJSONString(bodyMatch[1])
	}

	return title, body
}

func fixUnescapedJSONNewlines(jsonStr string) string {
	// Replaces literal unescaped newlines between quotes
	var buf strings.Builder
	inString := false
	escaped := false

	for i := 0; i < len(jsonStr); i++ {
		b := jsonStr[i]
		if escaped {
			buf.WriteByte(b)
			escaped = false
			continue
		}
		if b == '\\' {
			buf.WriteByte(b)
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
			buf.WriteByte(b)
			continue
		}
		if inString && b == '\n' {
			buf.WriteString("\\n")
			continue
		}
		if inString && b == '\r' {
			continue
		}
		if inString && b == '\t' {
			buf.WriteString("\\t")
			continue
		}
		buf.WriteByte(b)
	}
	return buf.String()
}

func unescapeJSONString(s string) string {
	var decoded string
	if err := json.Unmarshal([]byte(`"`+s+`"`), &decoded); err == nil {
		return decoded
	}
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\t`, "\t")
	return s
}
