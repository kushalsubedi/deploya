package detector

import (
	"os"
	"path/filepath"
	"strings"
)

type langRule struct {
	file    string
	lang    string
	runtime func(dir string) string
}

var langRules = []langRule{
	{file: "go.mod", lang: "go", runtime: goVersion},
	{file: "requirements.txt", lang: "python", runtime: pythonVersion},
	{file: "Pipfile", lang: "python", runtime: pythonVersion},
	{file: "pyproject.toml", lang: "python", runtime: pythonVersion},
	{file: "package.json", lang: "node", runtime: nodeVersion},
	{file: "pom.xml", lang: "java", runtime: func(string) string { return "17" }},
	{file: "build.gradle", lang: "java", runtime: func(string) string { return "17" }},
	{file: "Gemfile", lang: "ruby", runtime: func(string) string { return "3.2" }},
	{file: "Cargo.toml", lang: "rust", runtime: func(string) string { return "stable" }},
}

func DetectLanguage(dir string) (lang, runtime string) {
	for _, rule := range langRules {
		if fileExists(filepath.Join(dir, rule.file)) {
			return rule.lang, rule.runtime(dir)
		}
	}
	return "unknown", ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func goVersion(dir string) string {
	// Check go.mod file, if their is no file then it will return 1.21 as version which is a failsafe/fallback

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "1.21"
	}
	// check exact go "version" line in go.mod file
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimPrefix(line, "go ")
		}
	}
	return "1.21"
}

func pythonVersion(dir string) string {
	// Check .python-version file first
	if b, err := os.ReadFile(filepath.Join(dir, ".python-version")); err == nil {
		return strings.TrimSpace(string(b))
	}
	// Check pyproject.toml for requires-python
	if b, err := os.ReadFile(filepath.Join(dir, "pyproject.toml")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "requires-python") {
				parts := strings.Split(line, `"`)
				for _, p := range parts {
					p = strings.TrimSpace(p)
					p = strings.TrimLeft(p, ">=<~^")
					if len(p) > 0 && (p[0] == '2' || p[0] == '3') {
						return p
					}
				}
			}
		}
	}
	return "3.11"
}

func nodeVersion(dir string) string {
	if b, err := os.ReadFile(filepath.Join(dir, ".nvmrc")); err == nil {
		return strings.TrimSpace(strings.TrimLeft(string(b), "v"))
	}
	if b, err := os.ReadFile(filepath.Join(dir, ".node-version")); err == nil {
		return strings.TrimSpace(strings.TrimLeft(string(b), "v"))
	}
	return "20"
}
