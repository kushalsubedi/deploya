package generator

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"deploya/config"
	"deploya/releaserc"
)

//go:embed templates/github_actions.yaml.tmpl
var githubActionsTemplate string

//go:embed templates/github/PULL_REQUEST_TEMPLATE.md
var prTemplate string

//go:embed templates/github/ISSUE_TEMPLATE/bug_report.md
var bugReportTemplate string

//go:embed templates/github/ISSUE_TEMPLATE/feature_request.md
var featureRequestTemplate string

//go:embed templates/github/CHANGELOG.md
var changelogTemplate string

// TemplateData extends ProjectContext with extra derived fields for the template.
type TemplateData struct {
	config.ProjectContext
	HasPipfile   bool
	HasPyproject bool
	ImageName    string // always lowercase version of RepoName
}

// Generate renders the pipeline YAML for the given context.
func Generate(ctx config.ProjectContext, dir string) (string, error) {
	data := TemplateData{
		ProjectContext: ctx,
		HasPipfile:     fileExists(filepath.Join(dir, "Pipfile")),
		HasPyproject:   fileExists(filepath.Join(dir, "pyproject.toml")),
		ImageName:      strings.ToLower(ctx.RepoName),
	}

	tmpl, err := template.New("pipeline").Parse(githubActionsTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Write saves the generated pipeline to .github/workflows/ci.yml.
func Write(content, dir string) (string, error) {
	outDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	outPath := filepath.Join(outDir, "ci.yml")
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return "", err
	}

	return outPath, nil
}

// WriteReleasePipeline generates .github/workflows/release.yml.
// Full implementation comes when release package is built.
func WriteReleasePipeline(cfg releaserc.Config, dir string) (string, error) {
	outDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	content := buildReleasePipeline(cfg)
	outPath := filepath.Join(outDir, "release.yml")
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return "", err
	}

	return outPath, nil
}

// WriteGithubTemplates generates PR template, issue templates and CHANGELOG.md.
func WriteGithubTemplates(dir string) ([]string, error) {
	var written []string

	// .github/PULL_REQUEST_TEMPLATE.md
	prPath := filepath.Join(dir, ".github", "PULL_REQUEST_TEMPLATE.md")
	if err := writeFile(prPath, prTemplate); err != nil {
		return written, err
	}
	written = append(written, prPath)

	// .github/ISSUE_TEMPLATE/bug_report.md
	bugPath := filepath.Join(dir, ".github", "ISSUE_TEMPLATE", "bug_report.md")
	if err := writeFile(bugPath, bugReportTemplate); err != nil {
		return written, err
	}
	written = append(written, bugPath)

	// .github/ISSUE_TEMPLATE/feature_request.md
	featPath := filepath.Join(dir, ".github", "ISSUE_TEMPLATE", "feature_request.md")
	if err := writeFile(featPath, featureRequestTemplate); err != nil {
		return written, err
	}
	written = append(written, featPath)

	// CHANGELOG.md — only create if it doesn't exist yet
	changelogPath := filepath.Join(dir, "CHANGELOG.md")
	if !fileExists(changelogPath) {
		if err := writeFile(changelogPath, changelogTemplate); err != nil {
			return written, err
		}
		written = append(written, changelogPath)
	}

	return written, nil
}

func buildReleasePipeline(cfg releaserc.Config) string {
	return `name: Release

on:
  push:
    branches: [ "` + cfg.OnBranch + `" ]

jobs:
  release:
    name: Release
    runs-on: ubuntu-latest
    permissions:
      contents: write
      packages: write

    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "stable"

      - name: Install deploya
        run: go install github.com/yourusername/deploya@latest

      - name: Run deploya release
        run: deploya release
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
