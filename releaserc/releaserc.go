package releaserc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Release modes. Independent gives every package its own version line;
// locked keeps one version across the whole repo.
const (
	ModeIndependent = "independent"
	ModeLocked      = "locked"
)

type Config struct {
	OnBranch       string              `yaml:"on_branch"`
	FromBranch     string              `yaml:"from_branch"`
	CurrentVersion string              `yaml:"current_version"`
	TagPrefix      string              `yaml:"tag_prefix"`
	Archive        bool                `yaml:"archive"`
	Registry       string              `yaml:"registry"`
	GithubRepo     string              `yaml:"github_repo"`
	Mode           string              `yaml:"mode,omitempty"`
	Packages       []Package           `yaml:"packages,omitempty"`
	Groups         map[string][]string `yaml:"groups,omitempty"`
	Categories     Categories          `yaml:"categories"`
}

// Package is one independently releasable unit inside the repo.
// A repo with no packages is treated as a single root package.
type Package struct {
	Name           string   `yaml:"name"`
	Path           string   `yaml:"path"`
	TagPrefix      string   `yaml:"tag_prefix,omitempty"`
	CurrentVersion string   `yaml:"current_version,omitempty"`
	Archive        bool     `yaml:"archive,omitempty"`
	Registry       string   `yaml:"registry,omitempty"`
	DependsOn      []string `yaml:"depends_on,omitempty"`
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

// IsMonorepo reports whether per-package releases are configured.
func (c Config) IsMonorepo() bool { return len(c.Packages) > 0 }

// Locked reports whether all packages share one version line.
func (c Config) Locked() bool { return c.Mode == ModeLocked }

// Units returns the release units with every default filled in. A repo
// without a packages list yields a single unnamed root unit, which is what
// keeps single-project behaviour identical to before monorepo support.
func (c Config) Units() []Package {
	if !c.IsMonorepo() {
		return []Package{{
			Path:           ".",
			TagPrefix:      c.TagPrefix,
			CurrentVersion: c.CurrentVersion,
			Archive:        c.Archive,
			Registry:       c.Registry,
		}}
	}
	units := make([]Package, 0, len(c.Packages))
	for _, p := range c.Packages {
		units = append(units, p.withDefaults(c))
	}
	return units
}

// Unit returns the fully defaulted unit with the given name.
func (c Config) Unit(name string) (Package, bool) {
	for _, u := range c.Units() {
		if strings.EqualFold(u.Name, name) {
			return u, true
		}
	}
	return Package{}, false
}

// withDefaults inherits repo-level settings the package did not override.
// In locked mode every package shares the repo tag prefix and version,
// because there is only one version line.
func (p Package) withDefaults(c Config) Package {
	if p.Path == "" {
		p.Path = "."
	}
	if p.Registry == "" {
		p.Registry = c.Registry
	}
	if c.Locked() {
		p.TagPrefix = c.TagPrefix
		p.CurrentVersion = c.CurrentVersion
		return p
	}
	if p.TagPrefix == "" {
		p.TagPrefix = p.Name + "/v"
	}
	if p.CurrentVersion == "" {
		p.CurrentVersion = "v0.0.0"
	}
	return p
}

// Dir returns the package directory under the repo root.
func (p Package) Dir(root string) string {
	if p.Path == "" || p.Path == "." {
		return root
	}
	return filepath.Join(root, p.Path)
}

// ArtifactName is the base name for binaries and image suffixes. Root units
// fall back to the repo name so single-project releases are unchanged.
func (p Package) ArtifactName(githubRepo string) string {
	if p.Name != "" {
		return p.Name
	}
	return repoName(githubRepo)
}

// Label is how the unit is referred to in CLI output.
func (p Package) Label() string {
	if p.Name == "" {
		return "root"
	}
	return p.Name
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
	if cfg.Mode == "" {
		cfg.Mode = ModeIndependent
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

const savedHeader = `# .releaserc — managed by deploya. Edit freely; deploya only
# rewrites current_version after a release.
`

// Save writes the config back to .releaserc — used to update current_version
// after a release.
func Save(dir string, cfg Config) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("could not encode .releaserc: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, ".releaserc"), []byte(savedHeader+string(b)), 0o644)
}

// SetVersion records a released version against a unit. An empty name
// targets the repo-level version used by root and locked releases.
func (c *Config) SetVersion(name, version string) {
	if name == "" || c.Locked() {
		c.CurrentVersion = version
		return
	}
	for i := range c.Packages {
		if strings.EqualFold(c.Packages[i].Name, name) {
			c.Packages[i].CurrentVersion = version
			return
		}
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
	if cfg.Mode != ModeIndependent && cfg.Mode != ModeLocked {
		return fmt.Errorf(".releaserc: mode must be %q or %q, got %q", ModeIndependent, ModeLocked, cfg.Mode)
	}

	seen := map[string]bool{}
	for i, p := range cfg.Packages {
		if p.Name == "" {
			return fmt.Errorf(".releaserc: packages[%d] is missing a name", i)
		}
		if seen[strings.ToLower(p.Name)] {
			return fmt.Errorf(".releaserc: duplicate package name %q", p.Name)
		}
		seen[strings.ToLower(p.Name)] = true
		if strings.Contains(p.Path, "..") {
			return fmt.Errorf(".releaserc: package %q path must stay inside the repo", p.Name)
		}
	}
	for _, p := range cfg.Packages {
		for _, dep := range p.DependsOn {
			if !seen[strings.ToLower(dep)] {
				return fmt.Errorf(".releaserc: package %q depends_on unknown package %q", p.Name, dep)
			}
		}
	}
	for name, members := range cfg.Groups {
		if len(members) == 0 {
			return fmt.Errorf(".releaserc: group %q is empty", name)
		}
		for _, m := range members {
			if !seen[strings.ToLower(m)] {
				return fmt.Errorf(".releaserc: group %q references unknown package %q", name, m)
			}
		}
	}
	return nil
}
