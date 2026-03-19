package cmd

import (
	"fmt"
	"os"
)

const version = "0.1.0"

const usage = `deploya — zero-config CI/CD pipeline generator

Usage:
  deploya <command> [flags]

Commands:
  init        Detect project and generate GitHub Actions pipeline
  validate    Lint and validate the generated pipeline YAML
  preview     Dry-run: print the pipeline to stdout without writing files
  add         Add a new job or step to an existing pipeline

Flags:
  --version   Print version and exit
  --help      Show this help message

Examples:
  deploya init
  deploya preview
  deploya validate
  deploya add --job deploy
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
	case "--version", "version":
		fmt.Printf("deploya v%s\n", version)
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
