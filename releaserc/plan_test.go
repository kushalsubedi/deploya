package releaserc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── test repo helpers ───────────────────────────────────────────────────────

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// newRepo creates a git repo with two package directories.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "--quiet", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
	gitRun(t, dir, "config", "tag.gpgSign", "false")
	return dir
}

// commitFile writes a file under path and commits it with the given subject.
func commitFile(t *testing.T, dir, path, subject string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(subject+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "--quiet", "-m", subject)
}

// monoConfig is a two-package config: web depends on api.
func monoConfig() Config {
	cfg := DefaultConfig()
	cfg.GithubRepo = "owner/repo"
	cfg.Mode = ModeIndependent
	cfg.Packages = []Package{
		{Name: "api", Path: "apps/api", CurrentVersion: "v0.1.0"},
		{Name: "web", Path: "apps/web", CurrentVersion: "v0.1.0", DependsOn: []string{"api"}},
	}
	cfg.Groups = map[string][]string{"platform": {"api"}}
	return cfg
}

func findRelease(plan Plan, name string) (Release, bool) {
	for _, r := range plan.Releases {
		if r.Pkg.Name == name {
			return r, true
		}
	}
	return Release{}, false
}

// ── tests ───────────────────────────────────────────────────────────────────

// A commit under one package must not bump the other.
func TestPlanIndependentIsolatesPackages(t *testing.T) {
	dir := newRepo(t)
	commitFile(t, dir, "apps/api/main.go", "chore: init api")
	commitFile(t, dir, "apps/web/index.js", "chore: init web")
	gitRun(t, dir, "tag", "api/v0.1.0")
	gitRun(t, dir, "tag", "web/v0.1.0")
	commitFile(t, dir, "apps/api/handler.go", "feat: add handler")

	plan, err := BuildPlan(dir, monoConfig(), nil, "")
	if err != nil {
		t.Fatal(err)
	}

	api, ok := findRelease(plan, "api")
	if !ok {
		t.Fatalf("api was not released; plan=%+v", plan)
	}
	if api.Tag != "api/v0.2.0" {
		t.Errorf("api tag = %q, want api/v0.2.0", api.Tag)
	}
	if api.Version != "v0.2.0" {
		t.Errorf("api version = %q, want v0.2.0 (bare, for file and image names)", api.Version)
	}
	if api.PrevTag != "api/v0.1.0" {
		t.Errorf("api prev tag = %q — package tag lines must not mix", api.PrevTag)
	}
	if len(api.Commits) != 1 {
		t.Errorf("api picked up %d commits, want 1 (path filter leaking)", len(api.Commits))
	}
}

// web has no commits of its own but depends on api, so it rides along.
func TestPlanDependencyCascade(t *testing.T) {
	dir := newRepo(t)
	commitFile(t, dir, "apps/api/main.go", "chore: init api")
	commitFile(t, dir, "apps/web/index.js", "chore: init web")
	gitRun(t, dir, "tag", "api/v0.1.0")
	gitRun(t, dir, "tag", "web/v0.1.0")
	commitFile(t, dir, "apps/api/handler.go", "feat: add handler")

	plan, err := BuildPlan(dir, monoConfig(), nil, "")
	if err != nil {
		t.Fatal(err)
	}

	web, ok := findRelease(plan, "web")
	if !ok {
		t.Fatalf("web should cascade from api; plan=%+v", plan)
	}
	if web.Tag != "web/v0.1.1" {
		t.Errorf("web tag = %q, want web/v0.1.1 (patch bump)", web.Tag)
	}
	if len(web.Commits) != 0 {
		t.Errorf("cascaded release should carry no commits, got %d", len(web.Commits))
	}
	if !strings.Contains(web.Reason, "api") {
		t.Errorf("cascade reason should name the trigger, got %q", web.Reason)
	}
	// It must not also be reported as skipped.
	for _, s := range plan.Skipped {
		if s.Name == "web" {
			t.Error("web is both released and skipped")
		}
	}
}

// With no dependency link, an untouched package is skipped.
func TestPlanSkipsUnchangedPackage(t *testing.T) {
	dir := newRepo(t)
	commitFile(t, dir, "apps/api/main.go", "chore: init api")
	commitFile(t, dir, "apps/web/index.js", "chore: init web")
	gitRun(t, dir, "tag", "api/v0.1.0")
	gitRun(t, dir, "tag", "web/v0.1.0")
	commitFile(t, dir, "apps/api/handler.go", "feat: add handler")

	cfg := monoConfig()
	cfg.Packages[1].DependsOn = nil

	plan, err := BuildPlan(dir, cfg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(plan.Releases))
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Name != "web" {
		t.Errorf("web should be skipped, got %+v", plan.Skipped)
	}
}

// Locked mode collapses to one version covering every package path.
func TestPlanLockedSingleVersion(t *testing.T) {
	dir := newRepo(t)
	commitFile(t, dir, "apps/api/main.go", "chore: init api")
	commitFile(t, dir, "apps/web/index.js", "chore: init web")
	gitRun(t, dir, "tag", "v1.0.0")
	commitFile(t, dir, "apps/api/handler.go", "feat: add handler")

	cfg := monoConfig()
	cfg.Mode = ModeLocked
	cfg.CurrentVersion = "v1.0.0"

	plan, err := BuildPlan(dir, cfg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != 1 {
		t.Fatalf("locked mode must yield one release, got %d", len(plan.Releases))
	}
	rel := plan.Releases[0]
	if rel.Tag != "v1.1.0" {
		t.Errorf("locked tag = %q, want v1.1.0", rel.Tag)
	}
	if len(rel.Changed) != 1 || rel.Changed[0] != "api" {
		t.Errorf("changed packages = %v, want [api]", rel.Changed)
	}
}

// --only and --group narrow the selection.
func TestPlanSelection(t *testing.T) {
	dir := newRepo(t)
	commitFile(t, dir, "apps/api/main.go", "feat: api")
	commitFile(t, dir, "apps/web/index.js", "feat: web")

	cfg := monoConfig()
	cfg.Packages[1].DependsOn = nil

	only, err := BuildPlan(dir, cfg, []string{"web"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(only.Releases) != 1 || only.Releases[0].Pkg.Name != "web" {
		t.Errorf("--only web released %+v", only.Releases)
	}

	group, err := BuildPlan(dir, cfg, nil, "platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Releases) != 1 || group.Releases[0].Pkg.Name != "api" {
		t.Errorf("--group platform released %+v", group.Releases)
	}

	if _, err := BuildPlan(dir, cfg, nil, "nope"); err == nil {
		t.Error("expected error for unknown group")
	}
	if _, err := BuildPlan(dir, cfg, []string{"nope"}, ""); err == nil {
		t.Error("expected error for unknown package")
	}
}

// Running release twice must do nothing the second time: deploya's own
// release commit is not a reason to cut another release.
func TestPlanIgnoresOwnReleaseCommits(t *testing.T) {
	dir := newRepo(t)
	commitFile(t, dir, "apps/api/main.go", "feat: api")
	commitFile(t, dir, "apps/web/index.js", "feat: web")
	gitRun(t, dir, "tag", "api/v0.1.0")
	gitRun(t, dir, "tag", "web/v0.1.0")
	commitFile(t, dir, "apps/api/CHANGELOG.md", "chore: release api/v0.1.0 [skip ci]")

	cfg := monoConfig()
	cfg.Packages[1].DependsOn = nil

	plan, err := BuildPlan(dir, cfg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != 0 {
		t.Errorf("release commits must not trigger a release, got %+v", plan.Releases)
	}
}

// A repo with no packages still behaves exactly like before.
func TestPlanSingleProjectUnchanged(t *testing.T) {
	dir := newRepo(t)
	commitFile(t, dir, "main.go", "feat: something")

	cfg := DefaultConfig()
	cfg.GithubRepo = "owner/repo"
	cfg.Mode = ModeIndependent

	plan, err := BuildPlan(dir, cfg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != 1 {
		t.Fatalf("expected a single root release, got %d", len(plan.Releases))
	}
	rel := plan.Releases[0]
	if rel.Tag != "v0.1.0" {
		t.Errorf("root tag = %q, want v0.1.0", rel.Tag)
	}
	if rel.Pkg.Label() != "root" {
		t.Errorf("root unit label = %q", rel.Pkg.Label())
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	base := func() Config {
		c := DefaultConfig()
		c.GithubRepo = "owner/repo"
		c.Mode = ModeIndependent
		return c
	}

	cases := map[string]func(*Config){
		"bad mode":        func(c *Config) { c.Mode = "sometimes" },
		"duplicate name":  func(c *Config) { c.Packages = []Package{{Name: "a"}, {Name: "A"}} },
		"missing name":    func(c *Config) { c.Packages = []Package{{Path: "apps/a"}} },
		"escaping path":   func(c *Config) { c.Packages = []Package{{Name: "a", Path: "../a"}} },
		"unknown dep":     func(c *Config) { c.Packages = []Package{{Name: "a", DependsOn: []string{"ghost"}}} },
		"unknown group":   func(c *Config) { c.Groups = map[string][]string{"g": {"ghost"}} },
		"empty group":     func(c *Config) { c.Groups = map[string][]string{"g": {}} },
		"no github repo":  func(c *Config) { c.GithubRepo = "" },
		"bad github repo": func(c *Config) { c.GithubRepo = "norepo" },
	}
	for name, mutate := range cases {
		cfg := base()
		mutate(&cfg)
		if err := validate(cfg); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

// Packages and groups must survive a .releaserc write/read cycle.
func TestSaveLoadMonorepoRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfg := monoConfig()
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Packages) != 2 || got.Packages[0].Name != "api" {
		t.Fatalf("packages lost in roundtrip: %+v", got.Packages)
	}
	if got.Packages[1].DependsOn[0] != "api" {
		t.Errorf("depends_on lost in roundtrip: %+v", got.Packages[1])
	}
	if len(got.Groups["platform"]) != 1 {
		t.Errorf("groups lost in roundtrip: %+v", got.Groups)
	}
	if got.Mode != ModeIndependent {
		t.Errorf("mode = %q", got.Mode)
	}
}

// Defaults a package does not set are inherited from the repo config.
func TestUnitDefaults(t *testing.T) {
	cfg := monoConfig()
	cfg.Registry = "ghcr"
	cfg.Packages[0].CurrentVersion = ""
	cfg.Packages[0].TagPrefix = ""

	u, ok := cfg.Unit("api")
	if !ok {
		t.Fatal("api unit missing")
	}
	if u.TagPrefix != "api/v" {
		t.Errorf("tag prefix = %q, want api/v", u.TagPrefix)
	}
	if u.CurrentVersion != "v0.0.0" {
		t.Errorf("current version = %q", u.CurrentVersion)
	}
	if u.Registry != "ghcr" {
		t.Errorf("registry not inherited: %q", u.Registry)
	}

	// Locked mode shares one tag line across every package.
	cfg.Mode = ModeLocked
	cfg.TagPrefix = "v"
	cfg.CurrentVersion = "v2.0.0"
	u, _ = cfg.Unit("api")
	if u.TagPrefix != "v" || u.CurrentVersion != "v2.0.0" {
		t.Errorf("locked unit = %+v, want repo tag prefix and version", u)
	}
}
