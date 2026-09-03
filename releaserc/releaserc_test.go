package releaserc

import (
	"os"
	"path/filepath"
	"testing"
)

// The README documents a .releaserc full of inline comments — parsing it
// must work.
func TestLoadWithInlineComments(t *testing.T) {
	dir := t.TempDir()
	content := `on_branch: main # on which branch you want release to take place
from_branch: dev # this might not be needed
current_version: 0.1.1 # your current release version
tag_prefix: v # tag prefix eg: v0.0.1
archive: true # keep it until v1.0.0
registry: ghcr # this is optional
github_repo: kushalsubedi/deploya # your github repo
categories:
  features: [feat, feature]
  fixes: [fix, bugfix, bug]
  patches: [chore, refactor, perf, improvement]
  docs: [docs, doc]
`
	if err := os.WriteFile(filepath.Join(dir, ".releaserc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.CurrentVersion != "0.1.1" {
		t.Errorf("CurrentVersion = %q, want %q", cfg.CurrentVersion, "0.1.1")
	}
	if cfg.TagPrefix != "v" {
		t.Errorf("TagPrefix = %q, want %q", cfg.TagPrefix, "v")
	}
	if !cfg.Archive {
		t.Error("Archive = false, want true")
	}
	if cfg.GithubRepo != "kushalsubedi/deploya" {
		t.Errorf("GithubRepo = %q", cfg.GithubRepo)
	}
	if len(cfg.Categories.Features) != 2 || cfg.Categories.Features[0] != "feat" {
		t.Errorf("Categories.Features = %v", cfg.Categories.Features)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.GithubRepo = "owner/repo"
	cfg.CurrentVersion = "v2.3.4"
	cfg.Archive = true

	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.CurrentVersion != "v2.3.4" {
		t.Errorf("CurrentVersion = %q, want v2.3.4", got.CurrentVersion)
	}
	if !got.Archive {
		t.Error("Archive was dropped in Save/Load roundtrip")
	}
	if got.GithubRepo != "owner/repo" {
		t.Errorf("GithubRepo = %q", got.GithubRepo)
	}
}

func TestLoadValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".releaserc"), []byte("current_version: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("expected error for missing github_repo, got nil")
	}
}
