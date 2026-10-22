package releaserc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// OrgManifestFile is the org-level manifest deploya looks for.
const OrgManifestFile = "deploya.org.yml"

// defaultWorkspace is where repos without a local checkout are cloned.
const defaultWorkspace = ".deploya/repos"

// OrgManifest describes a set of repositories released together as one
// product portfolio.
type OrgManifest struct {
	Org       string    `yaml:"org"`
	Workspace string    `yaml:"workspace,omitempty"`
	Repos     []OrgRepo `yaml:"repos"`
}

// OrgRepo is one repository in the portfolio. Only `repo` is required —
// dir points at an existing checkout, otherwise the repo is cloned into the
// workspace. only/group narrow which packages inside that repo are released.
type OrgRepo struct {
	Repo  string   `yaml:"repo"`
	Dir   string   `yaml:"dir,omitempty"`
	Only  []string `yaml:"only,omitempty"`
	Group string   `yaml:"group,omitempty"`
}

// Name returns the repo part of "owner/repo".
func (r OrgRepo) Name() string { return repoName(r.Repo) }

// Matches reports whether a user-supplied selector names this repo, either
// as "owner/repo" or just "repo".
func (r OrgRepo) Matches(sel string) bool {
	sel = strings.TrimSpace(sel)
	return strings.EqualFold(sel, r.Repo) || strings.EqualFold(sel, r.Name())
}

// LoadOrg reads the org manifest from dir.
func LoadOrg(dir string) (OrgManifest, error) {
	path := filepath.Join(dir, OrgManifestFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return OrgManifest{}, fmt.Errorf("could not open %s: %w", OrgManifestFile, err)
	}

	var m OrgManifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return OrgManifest{}, fmt.Errorf("%s is not valid YAML: %w", OrgManifestFile, err)
	}
	if m.Workspace == "" {
		m.Workspace = defaultWorkspace
	}
	if len(m.Repos) == 0 {
		return OrgManifest{}, fmt.Errorf("%s: repos list is empty", OrgManifestFile)
	}
	for i, r := range m.Repos {
		if !strings.Contains(r.Repo, "/") {
			return OrgManifest{}, fmt.Errorf("%s: repos[%d].repo must be owner/repo, got %q", OrgManifestFile, i, r.Repo)
		}
	}
	return m, nil
}

// SaveOrg writes the manifest to dir.
func SaveOrg(dir string, m OrgManifest) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	header := "# " + OrgManifestFile + " — org-level release portfolio.\n" +
		"# `deploya org status` shows what is unreleased across every repo.\n" +
		"# `deploya org release` cuts them, isolating failures per repo.\n"
	return os.WriteFile(filepath.Join(dir, OrgManifestFile), []byte(header+string(b)), 0o644)
}

// Select narrows the manifest to the named repos. An empty selector keeps all.
func (m OrgManifest) Select(only []string) ([]OrgRepo, error) {
	if len(only) == 0 {
		return m.Repos, nil
	}
	var out []OrgRepo
	for _, sel := range only {
		sel = strings.TrimSpace(sel)
		if sel == "" {
			continue
		}
		found := false
		for _, r := range m.Repos {
			if r.Matches(sel) {
				out = append(out, r)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("repo %q is not in %s", sel, OrgManifestFile)
		}
	}
	return out, nil
}

// Checkout resolves a repo to a local working directory, cloning it into the
// workspace when no existing checkout is configured. root is the directory
// holding the manifest.
func (m OrgManifest) Checkout(root string, r OrgRepo, token string) (string, error) {
	if r.Dir != "" {
		dir := r.Dir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			return "", fmt.Errorf("%s: dir %q is not a git checkout", r.Repo, r.Dir)
		}
		return dir, nil
	}

	dir := filepath.Join(root, m.Workspace, r.Name())
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		// Already cloned — refresh refs so tag lookups are current.
		if err := runGit(dir, "fetch", "--all", "--tags", "--quiet"); err != nil {
			return "", fmt.Errorf("%s: fetch failed: %w", r.Repo, err)
		}
		return dir, nil
	}

	if token == "" {
		return "", fmt.Errorf("%s: no local dir configured and no token available to clone", r.Repo)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}

	clean := "https://github.com/" + r.Repo + ".git"
	authed := "https://x-access-token:" + token + "@github.com/" + r.Repo + ".git"
	cmd := exec.Command("git", "clone", "--quiet", authed, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Never echo the authenticated URL — it carries the token.
		return "", fmt.Errorf("%s: clone failed: %s", r.Repo, scrub(string(out), token))
	}
	// Drop the token from .git/config; it is only needed for the clone itself.
	if err := runGit(dir, "remote", "set-url", "origin", clean); err != nil {
		return "", fmt.Errorf("%s: could not reset remote url: %w", r.Repo, err)
	}
	return dir, nil
}

func scrub(s, token string) string {
	if token == "" {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.ReplaceAll(s, token, "***"))
}
