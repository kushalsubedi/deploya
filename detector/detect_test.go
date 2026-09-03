package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslateNodeCmd(t *testing.T) {
	tests := []struct {
		cmd, pm, want string
	}{
		{"npm test", "yarn", "yarn test"},
		{"npm test", "pnpm", "pnpm test"},
		{"npm run test", "pnpm", "pnpm test"},
		{"npm run build", "yarn", "yarn build"},
		{"npx jest", "pnpm", "pnpm exec jest"},
		{"npm test", "npm", "npm test"},
		{"npm run build", "npm-nolock", "npm run build"},
		{"", "pnpm", ""},
	}
	for _, tt := range tests {
		if got := translateNodeCmd(tt.cmd, tt.pm); got != tt.want {
			t.Errorf("translateNodeCmd(%q, %q) = %q, want %q", tt.cmd, tt.pm, got, tt.want)
		}
	}
}

func TestDetectPackageManager(t *testing.T) {
	dir := t.TempDir()
	if got := DetectPackageManager(dir); got != "npm-nolock" {
		t.Errorf("no lockfile: got %q, want npm-nolock", got)
	}
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644)
	if got := DetectPackageManager(dir); got != "npm" {
		t.Errorf("package-lock: got %q, want npm", got)
	}
	os.WriteFile(filepath.Join(dir, "yarn.lock"), []byte(""), 0o644)
	if got := DetectPackageManager(dir); got != "yarn" {
		t.Errorf("yarn.lock: got %q, want yarn", got)
	}
	os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(""), 0o644)
	if got := DetectPackageManager(dir); got != "pnpm" {
		t.Errorf("pnpm-lock: got %q, want pnpm", got)
	}
}

func TestDetectLanguage(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.22\n"), 0o644)
	lang, runtime := DetectLanguage(dir)
	if lang != "go" || runtime != "1.22" {
		t.Errorf("got %q %q, want go 1.22", lang, runtime)
	}
}
