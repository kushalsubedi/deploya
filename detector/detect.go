package detector

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kushalsubedi/deploya/config"
)

// Detect runs all detectors and returns a complete ProjectContext.
func Detect(dir string) config.ProjectContext {
	lang, runtime := DetectLanguage(dir)
	hasDocker, hasCompose := DetectDocker(dir)
	testCmd := DetectTestCommand(dir, lang)
	cloud := DetectCloud(dir)
	mainBranch := detectMainBranch(dir)
	repoName := detectRepoName(dir)

	// Framework + build command (only for node projects)
	framework := ""
	buildCommand := ""
	if lang == "node" {
		framework, buildCommand = DetectFramework(dir)
	}

	return config.ProjectContext{
		Language:     lang,
		Runtime:      runtime,
		Framework:    framework,
		HasDocker:    hasDocker,
		HasCompose:   hasCompose,
		TestCommand:  testCmd,
		BuildCommand: buildCommand,
		Cloud:        cloud,
		MainBranch:   mainBranch,
		RepoName:     repoName,
	}
}

func detectMainBranch(dir string) string {
	headFile := filepath.Join(dir, ".git", "HEAD")
	b, err := os.ReadFile(headFile)
	if err != nil {
		return "main"
	}
	// content looks like: "ref: refs/heads/main"
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
