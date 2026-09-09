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

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(PullRequestResult{
				ID:      101,
				Number:  42,
				Title:   body["title"].(string),
				HTMLURL: "https://github.com/kushalsubedi/deploya/pull/42",
				State:   "open",
				Head:    body["head"].(string),
				Base:    body["base"].(string),
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
