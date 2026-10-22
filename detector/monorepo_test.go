package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byName(pkgs []WorkspacePkg, name string) (WorkspacePkg, bool) {
	for _, p := range pkgs {
		if p.Name == name {
			return p, true
		}
	}
	return WorkspacePkg{}, false
}

func TestDetectPnpmWorkspace(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pnpm-workspace.yaml", "packages:\n  - 'apps/*'\n  - 'libs/**'\n")
	write(t, dir, "apps/web/package.json", `{"name":"@acme/web","dependencies":{"@acme/ui":"workspace:*","react":"18"}}`)
	write(t, dir, "apps/admin/package.json", `{"name":"@acme/admin"}`)
	write(t, dir, "libs/ui/package.json", `{"name":"@acme/ui"}`)

	pkgs := DetectWorkspace(dir)
	if len(pkgs) != 3 {
		t.Fatalf("expected 3 packages, got %d: %+v", len(pkgs), pkgs)
	}

	web, ok := byName(pkgs, "web")
	if !ok {
		t.Fatalf("web not detected: %+v", pkgs)
	}
	if web.Path != "apps/web" {
		t.Errorf("web path = %q", web.Path)
	}
	// Only sibling workspace packages count as dependencies, not react.
	if len(web.DependsOn) != 1 || web.DependsOn[0] != "ui" {
		t.Errorf("web deps = %v, want [ui]", web.DependsOn)
	}
}

func TestDetectNpmWorkspacesField(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"root","workspaces":["packages/*"]}`)
	write(t, dir, "packages/a/package.json", `{"name":"a"}`)
	write(t, dir, "packages/b/package.json", `{"name":"b","devDependencies":{"a":"*"}}`)

	pkgs := DetectWorkspace(dir)
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %+v", pkgs)
	}
	b, _ := byName(pkgs, "b")
	if len(b.DependsOn) != 1 || b.DependsOn[0] != "a" {
		t.Errorf("b deps = %v, want [a]", b.DependsOn)
	}
}

func TestDetectGoWorkspace(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.work", "go 1.22\n\nuse (\n\t./services/api\n\t./services/worker\n)\n")
	write(t, dir, "services/api/go.mod", "module github.com/acme/api\n\ngo 1.22\n")
	write(t, dir, "services/worker/go.mod", "module github.com/acme/worker\n\ngo 1.22\n\nrequire (\n\tgithub.com/acme/api v0.1.0\n\tgithub.com/spf13/cobra v1.0.0\n)\n")

	pkgs := DetectWorkspace(dir)
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %+v", pkgs)
	}
	worker, ok := byName(pkgs, "worker")
	if !ok {
		t.Fatalf("worker not detected: %+v", pkgs)
	}
	if worker.Lang != "go" {
		t.Errorf("worker lang = %q", worker.Lang)
	}
	if len(worker.DependsOn) != 1 || worker.DependsOn[0] != "api" {
		t.Errorf("worker deps = %v, want [api] (external modules must be ignored)", worker.DependsOn)
	}
}

func TestDetectCargoWorkspace(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[workspace]\nmembers = [\n  \"crates/core\",\n  \"crates/cli\",\n]\n")
	write(t, dir, "crates/core/Cargo.toml", "[package]\nname = \"core\"\n")
	write(t, dir, "crates/cli/Cargo.toml", "[package]\nname = \"cli\"\n")

	pkgs := DetectWorkspace(dir)
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 crates, got %+v", pkgs)
	}
}

// A single-package repo is not a monorepo.
func TestDetectWorkspaceIgnoresSingleProject(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"solo","workspaces":["packages/*"]}`)
	write(t, dir, "packages/only/package.json", `{"name":"only"}`)

	if pkgs := DetectWorkspace(dir); pkgs != nil {
		t.Errorf("one package is not a monorepo, got %+v", pkgs)
	}
}

func TestDetectWorkspaceNoManifest(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module solo\n")
	if pkgs := DetectWorkspace(dir); pkgs != nil {
		t.Errorf("expected nil for plain project, got %+v", pkgs)
	}
}
