package detector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type packageJSON struct {
	Scripts map[string]string `json:"scripts"`
}

// DetectTestCommand returns the best test command for the project.
func DetectTestCommand(dir, lang string) string {
	switch lang {
	case "go":
		return "go test ./..."
	case "python":
		return detectPythonTest(dir)
	case "node":
		return detectNodeTest(dir)
	case "java":
		if fileExists(filepath.Join(dir, "mvnw")) {
			return "./mvnw test"
		}
		if fileExists(filepath.Join(dir, "gradlew")) {
			return "./gradlew test"
		}
		return "mvn test"
	case "ruby":
		if fileExists(filepath.Join(dir, "spec")) {
			return "bundle exec rspec"
		}
		return "bundle exec rake test"
	case "rust":
		return "cargo test"
	}
	return ""
}

func detectPythonTest(dir string) string {
	if fileExists(filepath.Join(dir, "pytest.ini")) {
		return "pytest"
	}
	if fileExists(filepath.Join(dir, "conftest.py")) {
		return "pytest"
	}
	if b, err := os.ReadFile(filepath.Join(dir, "pyproject.toml")); err == nil {
		if strings.Contains(string(b), "[tool.pytest") {
			return "pytest"
		}
	}
	return "pytest"
}

func detectNodeTest(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "npm test"
	}
	var pkg packageJSON
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "npm test"
	}
	if script, ok := pkg.Scripts["test"]; ok {
		if strings.Contains(script, "vitest") {
			return "npm run test"
		}
		return "npm test"
	}
	if fileExists(filepath.Join(dir, "jest.config.js")) || fileExists(filepath.Join(dir, "jest.config.ts")) {
		return "npx jest"
	}
	return "npm test"
}
