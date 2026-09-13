package pr

import (
	"strings"
	"testing"
)

func TestParseRepoFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
		wantErr  bool
	}{
		{"git@github.com:kushalsubedi/deploya.git", "kushalsubedi/deploya", false},
		{"git@github.com:kushalsubedi/deploya", "kushalsubedi/deploya", false},
		{"https://github.com/kushalsubedi/deploya.git", "kushalsubedi/deploya", false},
		{"https://github.com/kushalsubedi/deploya", "kushalsubedi/deploya", false},
		{"ssh://git@github.com/kushalsubedi/deploya.git", "kushalsubedi/deploya", false},
		{"git://github.com/kushalsubedi/deploya.git", "kushalsubedi/deploya", false},
		{"", "", true},
		{"invalid-url", "", true},
	}

	for _, tt := range tests {
		got, err := ParseRepoFromURL(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseRepoFromURL(%q) err = %v, wantErr = %v", tt.url, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.expected {
			t.Errorf("ParseRepoFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestGenerateMarkdown(t *testing.T) {
	commits := []Commit{
		{
			SHA:    "1234567890abcdef",
			Short:  "1234567",
			Title:  "feat(cli): add deploya pr command",
			Author: "Kushal Subedi",
			Type:   "feat",
			Scope:  "cli",
		},
		{
			SHA:    "abcdef1234567890",
			Short:  "abcdef1",
			Title:  "fix(git): resolve base branch comparison",
			Author: "Kushal Subedi",
			Type:   "fix",
			Scope:  "git",
		},
		{
			SHA:    "5555555555555555",
			Short:  "5555555",
			Title:  "docs: document deploya pr",
			Author: "Kushal Subedi",
			Type:   "docs",
		},
	}

	res := GenerateMarkdown("feature/pr-command", "main", "kushalsubedi/deploya", commits, "")

	if !strings.Contains(res.Title, "pr command") && !strings.Contains(res.Title, "feat") {
		t.Errorf("Unexpected title: %q", res.Title)
	}

	if !strings.Contains(res.Body, "## 🎯 Overview") {
		t.Errorf("Missing Overview section in body")
	}
	if !strings.Contains(res.Body, "3 unmerged commit(s)") {
		t.Errorf("Commit count not reported accurately")
	}
	if !strings.Contains(res.Body, "✨ Features") {
		t.Errorf("Missing Features category")
	}
	if !strings.Contains(res.Body, "🐛 Bug Fixes") {
		t.Errorf("Missing Bug Fixes category")
	}
	if !strings.Contains(res.Body, "📝 Documentation") {
		t.Errorf("Missing Documentation category")
	}
	// Verify raw diff stat or git status file list is NOT present
	if strings.Contains(res.Body, "Diff Summary") {
		t.Errorf("Diff Summary section should not be included in concise PR")
	}
}

func TestParseGeminiPRResponse(t *testing.T) {
	t.Run("Valid JSON", func(t *testing.T) {
		raw := `{"title": "feat: awesome new feature", "body": "## Overview\nThis is a great PR."}`
		content, err := ParseGeminiPRResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content.Title != "feat: awesome new feature" {
			t.Errorf("expected title 'feat: awesome new feature', got %q", content.Title)
		}
		if !strings.Contains(content.Body, "Overview") {
			t.Errorf("expected body to contain 'Overview', got %q", content.Body)
		}
	})

	t.Run("JSON inside Markdown Code Block", func(t *testing.T) {
		raw := "```json\n{\n  \"title\": \"fix(ci): fix docker build\",\n  \"body\": \"## Summary\\nFixes docker issue.\"\n}\n```"
		content, err := ParseGeminiPRResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content.Title != "fix(ci): fix docker build" {
			t.Errorf("expected title 'fix(ci): fix docker build', got %q", content.Title)
		}
	})

	t.Run("JSON with unescaped literal newlines in body", func(t *testing.T) {
		raw := "{\n  \"title\": \"feat(ui): add modern dashboard\",\n  \"body\": \"## 🎯 Overview\nAdds a new dashboard component.\n\n## ✨ Key Changes\n- Beautiful charts\"\n}"
		content, err := ParseGeminiPRResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content.Title != "feat(ui): add modern dashboard" {
			t.Errorf("expected title 'feat(ui): add modern dashboard', got %q", content.Title)
		}
		if !strings.Contains(content.Body, "Key Changes") {
			t.Errorf("expected body to contain 'Key Changes', got %q", content.Body)
		}
	})

	t.Run("Conversational text around JSON", func(t *testing.T) {
		raw := "Here is your PR description:\n```json\n{\n  \"title\": \"refactor(core): streamline runner\",\n  \"body\": \"## 🎯 Overview\\nStreamlines core engine.\"\n}\n```\nLet me know if you need any adjustments!"
		content, err := ParseGeminiPRResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content.Title != "refactor(core): streamline runner" {
			t.Errorf("expected title 'refactor(core): streamline runner', got %q", content.Title)
		}
	})

	t.Run("Pure markdown without JSON", func(t *testing.T) {
		raw := "# feat(cli): introduce interactive pr\n\n## 🎯 Overview\nProvides interactive PR generation."
		content, err := ParseGeminiPRResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content.Title != "feat(cli): introduce interactive pr" {
			t.Errorf("expected title 'feat(cli): introduce interactive pr', got %q", content.Title)
		}
		if !strings.Contains(content.Body, "interactive PR generation") {
			t.Errorf("expected body to contain overview text, got %q", content.Body)
		}
	})
}
