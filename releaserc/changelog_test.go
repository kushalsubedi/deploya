package releaserc

import (
	"strings"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"feat: add login", "Add login"},
		{"fix(auth): token refresh", "Token refresh"},
		{"doc: update readme", "Update readme"},
		{"feat!: breaking api", "Breaking api"},
		{"plain message", "Plain message"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := cleanTitle(tt.in); got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsSkipCommit(t *testing.T) {
	if !isSkipCommit("chore: release v1.0.0 [skip ci]") {
		t.Error("release commit should be skipped")
	}
	if isSkipCommit("feat: add release notes page") {
		t.Error("normal commit should not be skipped")
	}
}

func TestCategorizeCommits(t *testing.T) {
	cats := DefaultConfig().Categories
	commits := []CommitInfo{
		{Title: "feat: new thing"},
		{Title: "fix: broken thing"},
		{Title: "docs: readme"},
		{Title: "chore: release v1.0.0 [skip ci]"}, // must be filtered
		{Title: "random commit"},
	}
	result := CategorizeCommits(commits, cats)

	total := 0
	for _, c := range result {
		total += len(c.Commits)
	}
	if total != 4 {
		t.Errorf("expected 4 categorized commits (release commit filtered), got %d", total)
	}
}

func TestGenerateNotesCompareLink(t *testing.T) {
	categories := []Category{
		{Name: "Features", Emoji: "✨", Commits: []CommitInfo{{Title: "feat: x", SHA: "abcdef1234567890"}}},
	}
	notes := GenerateNotes("v0.5.0", categories, "v0.4.0", "owner/repo", nil)
	if !strings.Contains(notes, "compare/v0.4.0...v0.5.0") {
		t.Errorf("compare link missing or malformed:\n%s", notes)
	}

	first := GenerateNotes("v0.1.0", categories, "", "owner/repo", nil)
	if strings.Contains(first, "compare/") {
		t.Error("first release should not contain a compare link")
	}
}

func TestGenerateNotesAssets(t *testing.T) {
	categories := []Category{
		{Name: "Features", Emoji: "✨", Commits: []CommitInfo{{Title: "feat: x", SHA: "abcdef1234567890"}}},
	}
	assets := &Assets{
		ImageRef: "ghcr.io/owner/repo:v0.6.0",
		Files: []AssetFile{
			{Name: "repo_v0.6.0_linux_amd64.tar.gz"},
			{Name: "checksums.txt"},
		},
	}
	notes := GenerateNotes("v0.6.0", categories, "v0.5.0", "owner/repo", assets)

	if !strings.Contains(notes, "### 📦 Assets") {
		t.Fatalf("assets section missing:\n%s", notes)
	}
	if !strings.Contains(notes, "docker pull ghcr.io/owner/repo:v0.6.0") {
		t.Errorf("image pull command missing:\n%s", notes)
	}
	if !strings.Contains(notes, "https://github.com/owner/repo/pkgs/container/repo") {
		t.Errorf("package page link missing:\n%s", notes)
	}
	if !strings.Contains(notes, "https://github.com/owner/repo/releases/download/v0.6.0/repo_v0.6.0_linux_amd64.tar.gz") {
		t.Errorf("download link missing or malformed:\n%s", notes)
	}

	// No assets → no section
	empty := GenerateNotes("v0.6.0", categories, "v0.5.0", "owner/repo", &Assets{})
	if strings.Contains(empty, "Assets") {
		t.Error("empty assets should not render a section")
	}
}
