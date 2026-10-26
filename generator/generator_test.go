package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kushalsubedi/deploya/config"
	"github.com/kushalsubedi/deploya/releaserc"
	"gopkg.in/yaml.v3"
)

// TestGenerateProducesValidYAML renders the pipeline for every language,
// registry and notify combination and checks the output parses as YAML.
func TestGenerateProducesValidYAML(t *testing.T) {
	languages := []struct {
		lang    string
		runtime string
		pm      string
	}{
		{"go", "1.22", ""},
		{"python", "3.11", ""},
		{"node", "20", "npm"},
		{"node", "20", "yarn"},
		{"node", "20", "pnpm"},
		{"node", "20", "npm-nolock"},
		{"java", "17", ""},
		{"ruby", "3.2", ""},
		{"rust", "stable", ""},
	}
	registries := []string{"none", "ghcr", "dockerhub", "ecr", "gcr"}
	notifies := []string{"none", "slack", "discord", "email"}

	for _, l := range languages {
		for _, reg := range registries {
			for _, n := range notifies {
				ctx := config.ProjectContext{
					Language:       l.lang,
					Runtime:        l.runtime,
					PackageManager: l.pm,
					HasDocker:      reg != "none",
					Registry:       reg,
					Notify:         n,
					TestCommand:    "echo test",
					MainBranch:     "main",
					RepoName:       "My-Repo",
				}
				out, err := Generate(ctx, t.TempDir())
				if err != nil {
					t.Fatalf("%s/%s/%s: Generate failed: %v", l.lang, reg, n, err)
				}

				var doc map[string]interface{}
				if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
					t.Errorf("%s/%s/%s: output is not valid YAML: %v\n%s", l.lang, reg, n, err, out)
					continue
				}
				if _, ok := doc["jobs"]; !ok {
					t.Errorf("%s/%s/%s: no jobs key in output", l.lang, reg, n)
				}
				if _, ok := doc["jobs"].(map[string]interface{})["summary"]; !ok {
					t.Errorf("%s/%s/%s: no summary job in output", l.lang, reg, n)
				}
				if strings.Contains(out, "IMAGE_NAME: My-Repo") {
					t.Errorf("%s/%s/%s: image name not lowercased", l.lang, reg, n)
				}
			}
		}
	}
}

func TestReleasePipelineIsValidYAML(t *testing.T) {
	cfg := releaserc.DefaultConfig()
	cfg.GithubRepo = "owner/repo"
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "services", "api")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.Packages = []releaserc.Package{{Name: "api", Path: "services/api"}}

	out := buildReleasePipeline(cfg, releaseImagesEnabled(cfg, dir))
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("release pipeline is not valid YAML: %v", err)
	}
	if !strings.Contains(out, "packages: write") {
		t.Error("release pipeline with a module Dockerfile should grant GHCR package write permission")
	}
	if !strings.Contains(out, "Write release summary") || !strings.Contains(out, "Version-tagged GHCR images") {
		t.Error("release pipeline should summarize the release and module image publishing")
	}
}

func TestReleasePipelineWithoutDockerHasSummaryButNoPackagePermission(t *testing.T) {
	cfg := releaserc.DefaultConfig()
	cfg.GithubRepo = "owner/repo"

	out := buildReleasePipeline(cfg, releaseImagesEnabled(cfg, t.TempDir()))
	if strings.Contains(out, "packages: write") {
		t.Error("release pipeline without Dockerfiles should not request package write permission")
	}
	if !strings.Contains(out, "Write release summary") {
		t.Error("release pipeline should always include a summary")
	}
}
