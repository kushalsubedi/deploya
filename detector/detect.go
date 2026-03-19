package detector

import (
	"deploya/config"
	"os"
	"path/filepath"
	"strings"
)

// Detect runs all detectors and returns a complete ProjectContext.
func Detect(dir string) config.ProjectContext {
	lang, runtime := DetectLanguage(dir)
	hasDocker, hasCompose := DetectDocker(dir)
	testCmd := DetectTestCommand(dir, lang)
	cloud := DetectCloud(dir)
	mainBranch := detectMainBranch(dir)
	repoName := detectRepoName(dir)

	return config.ProjectContext{
		Language:    lang,
		Runtime:     runtime,
		HasDocker:   hasDocker,
		HasCompose:  hasCompose,
		TestCommand: testCmd,
		Cloud:       cloud,
		MainBranch:  mainBranch,
		RepoName:    repoName,
	}
}

func detectMainBranch(dir string) string {
	headFile := filepath.Join(dir, ".git", "HEAD")
	b, err := os.ReadFile(headFile)
	if err != nil {
		return "main"
	}
	content := strings.TrimSpace(string(b))
	if strings.Contains(content, "master") {
		return "master"
	}
	return "main"
}

func detectRepoName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "my-project"
	}
	return filepath.Base(abs)
}
