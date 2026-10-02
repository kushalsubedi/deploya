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
	defaultGeminiModel = "gemini-2.0-flash"
	fallbackGeminiModel = "gemini-1.5-flash"
	geminiBaseURL       = "https://generativelanguage.googleapis.com/v1beta/models"
	openRouterBaseURL   = "https://openrouter.ai/api/v1/chat/completions"
	openRouterModel     = "liquid/lfm-2.5-2.6b:free"
)

// GeminiClient interacts with Google Gemini or OpenRouter REST API.
type GeminiClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewGeminiClient initializes a client with the given API key.
// If key is empty, it checks GEMINI_API_KEY, GOOGLE_API_KEY, and OPENROUTER_API_KEY environment variables.
func NewGeminiClient(key string) *GeminiClient {
	if key == "" {
		key = os.Getenv("GEMINI_API_KEY")
	}
	if key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
	}
	if key == "" {
		key = os.Getenv("OPENROUTER_API_KEY")
	}

	return &GeminiClient{
		apiKey:     strings.TrimSpace(key),
		httpClient: &http.Client{Timeout: 50 * time.Second},
	}
}

// IsAvailable returns true if an API key is present.
func (c *GeminiClient) IsAvailable() bool {
	return strings.TrimSpace(c.apiKey) != ""
}

// ProviderName returns the name of the AI service based on the API key prefix.
func (c *GeminiClient) ProviderName() string {
	if strings.HasPrefix(c.apiKey, "sk-or-") {
		return "OpenRouter AI"
	}
	return "Gemini AI"
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

type openRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
	} `json:"error,omitempty"`
}

// GeneratePR sends repository context, commits, and diff to Gemini AI or OpenRouter
// and returns a thorough, rich, and comprehensive Pull Request description.
func (c *GeminiClient) GeneratePR(ctx context.Context, repo, head, base string, commits []Commit, diffStat, diffPatch string) (*PRContent, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("AI API key is not configured")
	}

	prompt := buildPrompt(repo, head, base, commits, diffPatch)

	// If key starts with "sk-or-", route through OpenRouter
	if strings.HasPrefix(c.apiKey, "sk-or-") {
		return c.callOpenRouter(ctx, prompt)
	}

	// Try default Gemini model first, fallback to gemini-1.5-flash if needed
	content, err := c.callGemini(ctx, defaultGeminiModel, prompt)
	if err != nil && !strings.Contains(err.Error(), "API_KEY_INVALID") {
		// Attempt fallback model
		fallbackContent, fallbackErr := c.callGemini(ctx, fallbackGeminiModel, prompt)
		if fallbackErr == nil {
			return fallbackContent, nil
		}
	}

	return content, err
}

func (c *GeminiClient) callOpenRouter(ctx context.Context, prompt string) (*PRContent, error) {
	reqBody := openRouterRequest{
		Model: openRouterModel,
		Messages: []openRouterMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.2,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterBaseURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/kushalsubedi/deploya")
	req.Header.Set("X-Title", "Deploya")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter request error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read openrouter response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var oErr openRouterResponse
		if json.Unmarshal(respBytes, &oErr) == nil && oErr.Error != nil {
			return nil, fmt.Errorf("openrouter API error: %s", oErr.Error.Message)
		}
		return nil, fmt.Errorf("openrouter API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var oResp openRouterResponse
	if err := json.Unmarshal(respBytes, &oResp); err != nil {
		return nil, fmt.Errorf("failed to decode openrouter response: %w", err)
	}

	if len(oResp.Choices) == 0 {
		return nil, fmt.Errorf("openrouter returned no completions")
	}

	rawText := strings.TrimSpace(oResp.Choices[0].Message.Content)
	if rawText == "" {
		return nil, fmt.Errorf("openrouter returned empty message")
	}

	return ParseGeminiPRResponse(rawText)
}

func (c *GeminiClient) callGemini(ctx context.Context, model, prompt string) (*PRContent, error) {
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
			MaxOutputTokens:  6000,
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
	if len(diffPatch) > 30000 {
		diffPatch = diffPatch[:30000] + "\n\n[... diff truncated for length ...]"
	}

	return fmt.Sprintf(`You are an expert staff software engineer writing an in-depth, thorough, and highly articulate GitHub Pull Request description.
Carefully inspect the unmerged commits and the code diff below to synthesize a rich, high-context technical writeup explaining WHAT changed, WHY, and HOW to verify it.

Repository: %s
Source Branch (HEAD): %s
Target Branch (BASE): %s

Unmerged Commits:
%s

Code Diff:
%s

Instructions for generating the PR:
1. Title: A concise, standard Conventional Commit title (e.g. "feat(pr): add automated PR creation with AI").
2. Body: Generate an extensive, well-structured, and narrative Markdown body formatted with these exact sections:

## 🎯 Overview
Explain what this PR introduces and the high-level scope:
This pull request merges %d unmerged commit(s) from %s into %s.
Followed by a narrative summary of the feature or architectural change.

### What changed
Provide a detailed, narrative technical walkthrough organized by subsystem, package, or directory.
Mention the specific components, domain logic, APIs, routes, UI screens, tests, configuration, or CLI commands introduced or changed. Explain what each piece does rather than just naming them.

### Why
Explain the architectural rationale, motivation, and problem solved. Contrast with how it worked previously (e.g., prototype, tech debt, missing capability, refactor) and why this change improves the system.

### How it was verified / How to run
Provide concrete testing details:
- Commands to run tests and build steps (e.g. test suites executed, bundle/build verification).
- Step-by-step commands for reviewers to run or test locally (e.g. dev server commands, test commands, CLI commands with sample args).

## 🛠️ Key Changes
Categorize the commits into conventional sections with commit hashes, e.g.:
### ✨ Features
- feat: <commit title> (<short_hash>)
### 🐛 Bug Fixes (if any)
### ⚡ Performance & Refactoring (if any)

## 🧪 Verification & Checklist
- [x] Commits reviewed and verified
- [ ] Automated CI tests pass
- [ ] Code changes follow repository standards

---
<sub>Generated with [Deploya](https://github.com/kushalsubedi/deploya) and AI</sub>

CRITICAL GUIDELINES:
- DO NOT just dump raw git status line additions/deletions (+42/-10). Write informative, narrative technical descriptions of the changes.
- Be deep and substantive: analyze the functions, types, and logic visible in the diff and commits.
- Output ONLY valid JSON with keys "title" and "body".

Output JSON schema:
{
  "title": "...",
  "body": "..."
}
`, repo, head, base, commitList.String(), diffPatch, len(commits), head, base)
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

	// 2. Direct JSON unmarshal (handling string body or structured map)
	var rawMap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &rawMap); err == nil {
		if t, ok := rawMap["title"].(string); ok && t != "" {
			var bStr string
			if b, ok := rawMap["body"].(string); ok {
				bStr = b
			} else if bObj, ok := rawMap["body"].(map[string]interface{}); ok {
				var sb strings.Builder
				if wc, ok := bObj["what_changed"].(string); ok {
					sb.WriteString("### What changed\n" + wc + "\n\n")
				}
				if why, ok := bObj["why"].(string); ok {
					sb.WriteString("### Why\n" + why + "\n\n")
				}
				if hv, ok := bObj["how_verified"].(string); ok {
					sb.WriteString("### How it was verified\n" + hv + "\n\n")
				}
				bStr = sb.String()
			}
			if bStr != "" {
				return &PRContent{
					Title: cleanExtractedTitle(t),
					Body:  cleanExtractedBody(bStr),
				}, nil
			}
		}
	}

	// 3. Extract outermost JSON object { ... } if text had surrounding conversational text
	if firstBrace := strings.Index(raw, "{"); firstBrace != -1 {
		if lastBrace := strings.LastIndex(raw, "}"); lastBrace != -1 && lastBrace > firstBrace {
			candidateJSON := raw[firstBrace : lastBrace+1]
			var content PRContent
			if err := json.Unmarshal([]byte(candidateJSON), &content); err == nil && content.Title != "" && content.Body != "" {
				content.Title = cleanExtractedTitle(content.Title)
				content.Body = cleanExtractedBody(content.Body)
				return &content, nil
			}

			// Try fixing unescaped literal control characters in JSON strings
			sanitized := fixUnescapedJSONNewlines(candidateJSON)
			if err := json.Unmarshal([]byte(sanitized), &content); err == nil && content.Title != "" && content.Body != "" {
				content.Title = cleanExtractedTitle(content.Title)
				content.Body = cleanExtractedBody(content.Body)
				return &content, nil
			}
		}
	}

	// 4. Regex extraction of "title" and "body"
	title, body := extractFieldsRegex(raw)
	if title != "" && body != "" {
		return &PRContent{
			Title: cleanExtractedTitle(title),
			Body:  cleanExtractedBody(body),
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
		Body:  cleanExtractedBody(body),
	}, nil
}

func cleanExtractedTitle(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "title\": \"")
	t = strings.TrimPrefix(t, "\"title\": \"")
	t = strings.TrimPrefix(t, "title: ")
	t = strings.TrimPrefix(t, "Title: ")
	t = strings.Trim(t, `"'*`+"`")
	t = strings.TrimSuffix(t, ",")
	t = strings.Trim(t, `"'*`+"`")
	return strings.TrimSpace(t)
}

func cleanExtractedBody(b string) string {
	b = strings.TrimSpace(b)
	b = strings.TrimPrefix(b, "\"body\": \"")
	b = strings.TrimPrefix(b, "body\": \"")
	b = strings.TrimPrefix(b, "\"body\":")
	b = strings.TrimPrefix(b, "body:")
	b = strings.TrimSpace(b)
	if strings.HasPrefix(b, "\"") && strings.HasSuffix(b, "\"") && len(b) >= 2 {
		b = b[1 : len(b)-1]
	}
	if strings.Contains(b, `\n`) {
		b = strings.ReplaceAll(b, `\n`, "\n")
	}
	return strings.TrimSpace(b)
}

// extractFieldsRegex pulls out "title" and "body" values if standard JSON unmarshaling fails
func extractFieldsRegex(text string) (string, string) {
	titleRe := regexp.MustCompile(`"title"\s*:\s*"([^"\\]*(?:\\.[^"\\]*)*)"`)
	titleMatch := titleRe.FindStringSubmatch(text)
	var title string
	if len(titleMatch) > 1 {
		title = unescapeJSONString(titleMatch[1])
	}

	// Capture "body": "..." including multiline and escaped content
	bodyRe := regexp.MustCompile(`(?s)"body"\s*:\s*"(.*)"\s*}?$`)
	bodyMatch := bodyRe.FindStringSubmatch(text)
	var body string
	if len(bodyMatch) > 1 {
		body = unescapeJSONString(bodyMatch[1])
	} else {
		// Non-greedy fallback
		bodyRe2 := regexp.MustCompile(`(?s)"body"\s*:\s*"(.*?)"(?:\s*,|\s*}$)`)
		bodyMatch2 := bodyRe2.FindStringSubmatch(text)
		if len(bodyMatch2) > 1 {
			body = unescapeJSONString(bodyMatch2[1])
		}
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
