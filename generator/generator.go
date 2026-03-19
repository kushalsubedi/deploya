package generator

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"text/template"

	"deploya/config"
)

//go:embed github_actions.yaml.tmpl

var githubActionsTemplate string

// TemplateData extends ProjectContext with extra derived fields for the template.
type TemplateData struct {
	config.ProjectContext
	HasPipfile   bool
	HasPyproject bool
}

// Generate renders the pipeline YAML for the given context.
func Generate(ctx config.ProjectContext, dir string) (string, error) {
	data := TemplateData{
		ProjectContext: ctx,
		HasPipfile:     fileExists(filepath.Join(dir, "Pipfile")),
		HasPyproject:   fileExists(filepath.Join(dir, "pyproject.toml")),
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
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}

	outPath := filepath.Join(outDir, "ci.yml")
	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		return "", err
	}

	return outPath, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
