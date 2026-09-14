package releaserc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanArchives(t *testing.T) {
	files := PlanArchives("owner/myapp", "v1.2.3")

	// 5 platform archives + checksums
	if len(files) != 6 {
		t.Fatalf("expected 6 planned assets, got %d", len(files))
	}
	want := map[string]bool{
		"myapp_v1.2.3_linux_amd64.tar.gz":  false,
		"myapp_v1.2.3_linux_arm64.tar.gz":  false,
		"myapp_v1.2.3_darwin_amd64.tar.gz": false,
		"myapp_v1.2.3_darwin_arm64.tar.gz": false,
		"myapp_v1.2.3_windows_amd64.zip":   false,
		"checksums.txt":                    false,
	}
	for _, f := range files {
		if _, ok := want[f.Name]; !ok {
			t.Errorf("unexpected asset name %q", f.Name)
		}
		want[f.Name] = true
		if f.Path != "" {
			t.Errorf("planned asset %q should have no local path", f.Name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing planned asset %q", name)
		}
	}
}

func TestBuildArchivesRequiresGoProject(t *testing.T) {
	dir := t.TempDir()
	if _, err := BuildArchives(dir, "owner/repo", "v1.0.0"); err == nil {
		t.Error("expected error for non-Go project")
	}
}

func TestPlanImageRef(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{GithubRepo: "Owner/MyRepo", Registry: "ghcr"}

	// No Dockerfile → no image
	if ref := PlanImageRef(dir, cfg, "v1.0.0"); ref != "" {
		t.Errorf("expected empty ref without Dockerfile, got %q", ref)
	}

	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// GHCR refs must be lowercase
	if ref := PlanImageRef(dir, cfg, "v1.0.0"); ref != "ghcr.io/owner/myrepo:v1.0.0" {
		t.Errorf("unexpected image ref %q", ref)
	}

	// Unsupported registry → no image
	cfg.Registry = "dockerhub"
	if ref := PlanImageRef(dir, cfg, "v1.0.0"); ref != "" {
		t.Errorf("expected empty ref for unsupported registry, got %q", ref)
	}
}
