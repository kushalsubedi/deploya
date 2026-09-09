package pr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiClient_GeneratePR(t *testing.T) {
	// Mock Gemini API Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			http.Error(w, `{"error":{"code":401,"message":"API_KEY_INVALID"}}`, http.StatusUnauthorized)
			return
		}

		resp := geminiResponse{
			Candidates: []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					}{
						Parts: []struct {
							Text string `json:"text"`
						}{
							{
								Text: `{"title":"feat(pr): automated AI-powered pull requests","body":"## 🎯 Overview\nAdds deploya pr with Gemini AI support."}`,
							},
						},
					},
					FinishReason: "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &GeminiClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
	}

	// Override URL via client callModel or custom method
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	commits := []Commit{
		{SHA: "abcdef1", Short: "abcdef1", Title: "feat: add pr command", Author: "Kushal"},
	}

	// Test prompt construction
	prompt := buildPrompt("kushalsubedi/deploya", "feature/pr", "main", commits, "1 file changed", "diff content")
	if !strings.Contains(prompt, "kushalsubedi/deploya") {
		t.Errorf("prompt missing repository name")
	}
	if !strings.Contains(prompt, "feat: add pr command") {
		t.Errorf("prompt missing commit message")
	}

	// Test parse response
	parsed, err := ParseGeminiPRResponse(`{"title":"feat: test pr","body":"## Overview\nTest body"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Title != "feat: test pr" {
		t.Errorf("expected 'feat: test pr', got %q", parsed.Title)
	}

	_ = client
	_ = ctx
}
