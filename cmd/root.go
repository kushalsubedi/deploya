package cmd

import (
	"fmt"
	"os"
)

// Version is stamped at build time via:
//
//	go build -ldflags "-X github.com/kushalsubedi/deploya/cmd.Version=v1.2.3"
var Version = "dev"

const usage = `deploya — zero-config CI/CD pipeline generator

Usage:
  deploya <command> [flags]

Commands:
  init        Detect project and generate GitHub Actions pipeline
  validate    Lint and validate the generated pipeline YAML
  preview     Dry-run: print the pipeline to stdout without writing files
  add         Add a new job or step to an existing pipeline
  release     Cut a new release — bump version, generate changelog, publish to GitHub
  pr          Create a GitHub Pull Request with unmerged commits and AI summary

Flags:
  --version   Print version and exit
  --help      Show this help message

Examples:
  deploya init
  deploya preview
  deploya validate
  deploya add --job deploy
  deploya pr
  deploya pr --base main --draft
  deploya pr --dry-run
`

func Execute() error {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		return nil
	}

	switch os.Args[1] {
	case "init":
		return runInit(os.Args[2:])
	case "validate":
		return runValidate(os.Args[2:])
	case "preview":
		return runPreview(os.Args[2:])
	case "add":
		return runAdd(os.Args[2:])
	case "release":
		return runRelease(os.Args[2:])
	case "pr":
		return runPR(os.Args[2:])

	case "--version", "-v", "version":
		fmt.Printf("deploya %s\n", Version)
		return nil
	case "--help", "help", "-h":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", os.Args[1])
		fmt.Print(usage)
		return fmt.Errorf("unknown command: %q", os.Args[1])
	}
}
