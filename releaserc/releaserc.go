package releaserc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	OnBranch       string     `yaml:"on_branch"`
	FromBranch     string     `yaml:"from_branch"`
	CurrentVersion string     `yaml:"current_version"`
	TagPrefix      string     `yaml:"tag_prefix"`
	Archive        bool       `yaml:"archive"`
	Registry       string     `yaml:"registry"`
	GithubRepo     string     `yaml:"github_repo"`
	Categories     Categories `yaml:"categories"`
}

type Categories struct {
	Features []string `yaml:"features"`
	Fixes    []string `yaml:"fixes"`
	Patches  []string `yaml:"patches"`
	Docs     []string `yaml:"docs"`
}

func DefaultConfig() Config {
	return Config{
		OnBranch:       "main",
		FromBranch:     "develop",
		CurrentVersion: "v0.0.0",
		TagPrefix:      "v",
		Archive:        false,
		Registry:       "ghcr",

		Categories: Categories{
			Features: []string{"feat", "feature"},
			Fixes:    []string{"fix", "bugfix", "bug"},
			Patches:  []string{"chore", "refactor", "perf", "improvement"},
			Docs:     []string{"docs", "doc"},
		},
	}
}

func Load(dir string) (Config, error) {
	path := filepath.Join(dir, ".releaserc")
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("could not open .releaserc: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf(".releaserc is not valid YAML: %w", err)
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Save writes the config back to .releaserc — used to update current_version after a release.
func Save(dir string, cfg Config) error {
	path := filepath.Join(dir, ".releaserc")

	var sb strings.Builder
	sb.WriteString("on_branch: " + cfg.OnBranch + "\n")
	sb.WriteString("from_branch: " + cfg.FromBranch + "\n")
	sb.WriteString("current_version: " + cfg.CurrentVersion + "\n")
	sb.WriteString("tag_prefix: " + cfg.TagPrefix + "\n")
	sb.WriteString(fmt.Sprintf("archive: %v\n", cfg.Archive))
	sb.WriteString("registry: " + cfg.Registry + "\n")
	sb.WriteString("github_repo: " + cfg.GithubRepo + "\n")
	sb.WriteString("categories:\n")
	sb.WriteString("  features: [" + strings.Join(cfg.Categories.Features, ", ") + "]\n")
	sb.WriteString("  fixes: [" + strings.Join(cfg.Categories.Fixes, ", ") + "]\n")
	sb.WriteString("  patches: [" + strings.Join(cfg.Categories.Patches, ", ") + "]\n")
	sb.WriteString("  docs: [" + strings.Join(cfg.Categories.Docs, ", ") + "]\n")

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func validate(cfg Config) error {
	if cfg.GithubRepo == "" {
		return fmt.Errorf(".releaserc: github_repo is required (format: owner/repo)")
	}
	if !strings.Contains(cfg.GithubRepo, "/") {
		return fmt.Errorf(".releaserc: github_repo must be in format owner/repo")
	}
	if cfg.CurrentVersion == "" {
		return fmt.Errorf(".releaserc: current_version is required")
	}
	return nil
}
