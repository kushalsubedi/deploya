package cmd

import (
	"flag"
	"fmt"

	"github.com/kushalsubedi/deploya/detector"
	"github.com/kushalsubedi/deploya/generator"
)

// runPreview renders the CI pipeline to stdout without writing any files
// and without prompting — sensible defaults are used for choices.
func runPreview(args []string) error {
	fs := flag.NewFlagSet("preview", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory to scan")
	registry := fs.String("registry", "", "Container registry: ghcr, dockerhub, ecr, gcr")
	notify := fs.String("notify", "none", "Notification channel: slack, discord, email, none")
	fs.Usage = func() {
		fmt.Println(`Usage: deploya preview [flags]

Dry-run: prints the pipeline that 'deploya init' would generate, without
writing files or asking questions.

Flags:`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := detector.Detect(*dir)
	ctx.Notify = *notify

	if *registry != "" {
		ctx.Registry = *registry
	} else if ctx.HasDocker {
		// Same smart default init would suggest
		switch ctx.Cloud {
		case "aws":
			ctx.Registry = "ecr"
		case "gcp":
			ctx.Registry = "gcr"
		default:
			ctx.Registry = "ghcr"
		}
	} else {
		ctx.Registry = "none"
	}

	content, err := generator.Generate(ctx, *dir)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	fmt.Print(content)
	return nil
}
