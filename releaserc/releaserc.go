package releaserc

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	OnBranch       string
	FromBranch     string
	CurrentVersion string
	TagPrefix      string
	Archive        bool
	Registry       string
	GithubRepo     string
	Categories     Categories
}

type Categories struct {
	Features []string
	Fixes    []string
	Patches  []string
	Docs     []string
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
			Patches:  []string{"chore", "refactor", "pref", "improvement"},
			Docs:     []string{"docs", "doc"},
		},
	}
}

func Load(dir string) (Config, error) {
	path := dir + "/.releaserc"
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("could not open .releaserc: %w", err)
	}
	defer f.Close()

	cfg := DefaultConfig()

	scanner := bufio.NewScanner(f)
	var currentSection string

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Detect section headers (e.g. "categories:")
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
			currentSection = strings.TrimSuffix(trimmed, ":")
			continue
		}

		// Parse list items under a section (e.g. "  features: [feat, feature]")
		if currentSection == "categories" {
			parseCategory(&cfg, trimmed)
			continue
		}

		// Reset section when we hit a top-level key
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			currentSection = ""
		}

		// Parse top-level key: value
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "on_branch":
			cfg.OnBranch = val
		case "from_branch":
			cfg.FromBranch = val
		case "current_version":
			cfg.CurrentVersion = val
		case "tag_prefix":
			cfg.TagPrefix = val
		case "archive":
			cfg.Archive = val == "true"
		case "registry":
			cfg.Registry = val
		case "github_repo":
			cfg.GithubRepo = val
		}
	}

	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("error reading .releaserc: %w", err)
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Save writes the config back to .releaserc — used to update current_version after a release.
func Save(dir string, cfg Config) error {
	path := dir + "/.releaserc"

	var sb strings.Builder
	sb.WriteString("on_branch: " + cfg.OnBranch + "\n")
	sb.WriteString("from_branch: " + cfg.FromBranch + "\n")
	sb.WriteString("current_version: " + cfg.CurrentVersion + "\n")
	sb.WriteString("tag_prefix: " + cfg.TagPrefix + "\n")
	sb.WriteString("archive: " + boolStr(cfg.Archive) + "\n")
	sb.WriteString("registry: " + cfg.Registry + "\n")
	sb.WriteString("github_repo: " + cfg.GithubRepo + "\n")
	sb.WriteString("categories:\n")
	sb.WriteString("  features: [" + strings.Join(cfg.Categories.Features, ", ") + "]\n")
	sb.WriteString("  fixes: [" + strings.Join(cfg.Categories.Fixes, ", ") + "]\n")
	sb.WriteString("  patches: [" + strings.Join(cfg.Categories.Patches, ", ") + "]\n")
	sb.WriteString("  docs: [" + strings.Join(cfg.Categories.Docs, ", ") + "]\n")

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func parseCategory(cfg *Config, line string) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])

	// Strip brackets
	val = strings.Trim(val, "[]")
	items := []string{}
	for _, item := range strings.Split(val, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}

	switch key {
	case "features":
		cfg.Categories.Features = items
	case "fixes":
		cfg.Categories.Fixes = items
	case "patches":
		cfg.Categories.Patches = items
	case "docs":
		cfg.Categories.Docs = items
	}
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

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
