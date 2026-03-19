package cmd

import (
	"flag"
	"fmt"

	"deploya/config"
	"deploya/detector"
	"deploya/generator"
)

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory to scan")
	fs.Usage = func() {
		fmt.Println(`Usage: deploya init [flags]

Detects your project and generates a GitHub Actions pipeline.

Flags:`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Println(" Scanning project...")
	ctx := detector.Detect(*dir)
	printSummary(ctx)

	fmt.Println("\n Generating GitHub Actions pipeline...")
	content, err := generator.Generate(ctx, *dir)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	outPath, err := generator.Write(content, *dir)
	if err != nil {
		return fmt.Errorf("failed to write pipeline: %w", err)
	}

	fmt.Printf("\nPipeline written to: %s\n", outPath)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the generated file")
	fmt.Println("  2. Add required secrets in GitHub → Settings → Secrets")
	fmt.Println("  3. git add .github && git commit -m 'ci: add deploya pipeline'")
	fmt.Println("  4. Push and watch your pipeline run 🚀")
	return nil
}

func printSummary(ctx config.ProjectContext) {
	fmt.Println("\nDetection results:")
	fmt.Printf("   Language     : %s %s\n", ctx.Language, ctx.Runtime)
	fmt.Printf("   Docker       : %v (compose: %v)\n", ctx.HasDocker, ctx.HasCompose)
	fmt.Printf("   Test command : %s\n", ctx.TestCommand)
	fmt.Printf("   Cloud        : %s\n", ctx.Cloud)
	fmt.Printf("   Main branch  : %s\n", ctx.MainBranch)
	fmt.Printf("   Repo name    : %s\n", ctx.RepoName)
}
