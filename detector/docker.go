package detector

import (
	"os"
	"path/filepath"
)

func DetectDocker(dir string) (hasDocker, hasCompose bool) {
	dockerfiles := []string{
		"Dockerfile",
		"dockerfile",
		"Dockerfile.prod",
		"Dockerfile.production",
	}

	for _, f := range dockerfiles {
		if fileExists(filepath.Join(dir, f)) {
			hasDocker = true
			break
		}
	}
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"docker-compose.prod.yml",
		"docker-compose.production.yml",
		"compose.yml",
		"compose.yaml",
	}
	for _, f := range composeFiles {
		if fileExists(filepath.Join(dir, f)) {
			hasCompose = true
			break
		}
	}
	return
}

// DetectDockerignore checks if .dockerignore exists.
func DetectDockerignore(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".dockerignore"))
	return err == nil
}
