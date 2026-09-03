package detector

import "path/filepath"

// DetectPackageManager picks the Node package manager from the lockfile.
// "npm-nolock" means package.json exists but no lockfile — `npm ci` would fail.
func DetectPackageManager(dir string) string {
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(dir, "package-lock.json")):
		return "npm"
	}
	return "npm-nolock"
}
