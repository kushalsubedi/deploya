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

	// Framework, build command and package manager (only for node projects)
	framework := ""
	buildCommand := ""
	packageManager := ""
	if lang == "node" {
		framework, buildCommand = DetectFramework(dir)
		packageManager = DetectPackageManager(dir)
		buildCommand = translateNodeCmd(buildCommand, packageManager)
		testCmd = translateNodeCmd(testCmd, packageManager)
	}

	return config.ProjectContext{
		Language:       lang,
		Runtime:        runtime,
		Framework:      framework,
		PackageManager: packageManager,
		HasDocker:      hasDocker,
		HasCompose:     hasCompose,
		TestCommand:    testCmd,
		BuildCommand:   buildCommand,
		Cloud:          cloud,
		MainBranch:     mainBranch,
		RepoName:       repoName,
	}
}

// translateNodeCmd rewrites npm-flavoured commands for the detected
// package manager so generated pipelines don't mix npm with yarn/pnpm.
func translateNodeCmd(cmd, pm string) string {
	if cmd == "" {
		return cmd
	}
	var runner, exec string
	switch pm {
	case "yarn":
		runner, exec = "yarn", "yarn"
	case "pnpm":
		runner, exec = "pnpm", "pnpm exec"
	default:
		return cmd
	}
	switch {
	case cmd == "npm test":
		return runner + " test"
	case strings.HasPrefix(cmd, "npm run "):
		return runner + " " + strings.TrimPrefix(cmd, "npm run ")
	case strings.HasPrefix(cmd, "npx "):
		return exec + " " + strings.TrimPrefix(cmd, "npx ")
	}
	return cmd
}

func detectMainBranch(dir string) string {
	headFile := filepath.Join(dir, ".git", "HEAD")
	b, err := os.ReadFile(headFile)
	if err != nil {
		return "main"
	}
	// content looks like: "ref: refs/heads/main"
	content := strings.TrimSpace(string(b))
	branch := strings.TrimPrefix(content, "ref: refs/heads/")
	if branch == "master" {
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
