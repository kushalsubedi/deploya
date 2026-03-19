package detector

import (
	"os"
	"path/filepath"
	"strings"
)

func detectFromTerraform(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}

		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		s := string(b)
		if strings.Contains(s, `provider "aws"`) || strings.Contains(s, "aws_") {
			return "aws"
		}
		if strings.Contains(s, `provider "google"`) || strings.Contains(s, "google_") {
			return "gcp"
		}
		if strings.Contains(s, `provider "azurerm"`) {
			return "azure"
		}
	}
	return ""
}

func detectFromServerless(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "serverless.yml"))
	if err != nil {
		return ""
	}
	s := string(b)
	if strings.Contains(s, "provider: aws") || strings.Contains(s, "name: aws") {
		return "aws"
	}
	if strings.Contains(s, "provider: google") || strings.Contains(s, "name: google") {
		return "gcp"
	}
	return ""
}

func detectFromEnvFiles(dir string) string {
	envFiles := []string{".env", ".env.example", ".env.sample"}
	for _, f := range envFiles {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		s := string(b)
		if strings.Contains(s, "AWS_") {
			return "aws"
		}
		if strings.Contains(s, "GOOGLE_CLOUD") || strings.Contains(s, "GCP_") {
			return "gcp"
		}
		if strings.Contains(s, "AZURE_") {
			return "azure"
		}
	}
	return ""
}
func DetectCloud(dir string) string {
	if cloud := detectFromTerraform(dir); cloud != "" {
		return cloud
	}

	if cloud := detectFromServerless(dir); cloud != "" {
		return cloud
	}

	if fileExists(filepath.Join(dir, "samconfig.toml")) {
		return "aws"
	}
	if fileExists(filepath.Join(dir, "template.yaml")) {
		if b, _ := os.ReadFile(filepath.Join(dir, "template.yaml")); strings.Contains(string(b), "AWSTemplateFormatVersion") {
			return "aws"
		}
	}

	// Check GCP app.yaml
	if fileExists(filepath.Join(dir, "app.yaml")) {
		if b, _ := os.ReadFile(filepath.Join(dir, "app.yaml")); strings.Contains(string(b), "runtime:") {
			return "gcp"
		}
	}

	// Check .env files for cloud env var hints
	if cloud := detectFromEnvFiles(dir); cloud != "" {
		return cloud
	}

	return "none"
}
