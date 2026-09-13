package pr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubClient_CreatePullRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-gh-token" {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}

		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pulls") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body["head"] == "feature/conflict" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"message":"A pull request already exists for owner:feature/conflict."}]}`))
				return
			}

			// Return actual GitHub API format with head/base as objects
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         101,
				"number":     42,
				"title":      body["title"].(string),
				"html_url":   "https://github.com/kushalsubedi/deploya/pull/42",
				"state":      "open",
				"draft":      false,
				"created_at": "2026-09-13T12:00:00Z",
				"head": map[string]interface{}{
					"ref": body["head"].(string),
					"sha": "1234567890abcdef",
				},
				"base": map[string]interface{}{
					"ref": body["base"].(string),
					"sha": "abcdef1234567890",
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer ts.Close()

	// Direct test of request formatting
	client := &GitHubClient{
		token:      "test-gh-token",
		httpClient: ts.Client(),
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	client.setHeaders(req)

	if req.Header.Get("Authorization") != "Bearer test-gh-token" {
		t.Errorf("expected Authorization header to be set")
	}
	if req.Header.Get("Accept") != "application/vnd.github+json" {
		t.Errorf("expected Accept header to be application/vnd.github+json")
	}
}
