package cmd

import (
	"flag"
	"fmt"

	"deploya/config"
	"deploya/detector"
	"deploya/generator"
	"deploya/prompt"
)

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory to scan")
	registry := fs.String("registry", "", "Container registry: ghcr, dockerhub, ecr, gcr")
	notify := fs.String("notify", "", "Notification channel: slack, discord, email, none")
	fs.Usage = func() {
		fmt.Println(`Usage: deploya init [flags]
 
Detects your project and generates a GitHub Actions pipeline.
 
Flags:`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// ── Detect project ─────────────────────────────────────────────
	fmt.Println(" Scanning project...")
	ctx := detector.Detect(*dir)
	printSummary(ctx)

	// ── Registry choice ────────────────────────────────────────────
	if *registry != "" {
		ctx.Registry = *registry
	} else if ctx.HasDocker {
		ctx.Registry = askRegistry(ctx.Cloud)
	} else {
		ctx.Registry = "none"
	}

	// ── Framework confirmation (node only) ─────────────────────────
	if ctx.Language == "node" && ctx.Framework != "" && ctx.Framework != "plain" {
		fmt.Printf("\n  Detected framework: %s", ctx.Framework)
		if !prompt.Confirm("Is this correct?", true) {
			ctx.Framework = askFramework()
		}
	}

	// ── Notification choice ────────────────────────────────────────
	if *notify != "" {
		ctx.Notify = *notify
	} else {
		ctx.Notify = askNotify()
	}

	// ── Generate pipeline ──────────────────────────────────────────
	fmt.Println("\n Generating GitHub Actions pipeline...")
	content, err := generator.Generate(ctx, *dir)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	outPath, err := generator.Write(content, *dir)
	if err != nil {
		return fmt.Errorf("failed to write pipeline: %w", err)
	}

	// ── Summary report ─────────────────────────────────────────────
	printReport(ctx, outPath)
	return nil
}

func askRegistry(cloud string) string {
	choices := []prompt.Choice{
		{Key: "ghcr", Label: "GHCR — GitHub Container Registry (free, no extra secrets needed)"},
		{Key: "dockerhub", Label: "Docker Hub (needs DOCKERHUB_USERNAME + DOCKERHUB_TOKEN secrets)"},
		{Key: "ecr", Label: "AWS ECR (needs AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY secrets)"},
		{Key: "gcr", Label: "GCP GCR (needs GCP_SA_KEY + GCP_PROJECT_ID secrets)"},
	}

	// Smart default based on detected cloud
	defaultKey := "ghcr"
	if cloud == "aws" {
		defaultKey = "ecr"
	} else if cloud == "gcp" {
		defaultKey = "gcr"
	}

	return prompt.Select("Where do you want to push your Docker image?", choices, defaultKey)
}

func askFramework() string {
	choices := []prompt.Choice{
		{Key: "nextjs", Label: "Next.js"},
		{Key: "react", Label: "React (CRA / Vite)"},
		{Key: "vue", Label: "Vue"},
		{Key: "svelte", Label: "Svelte / SvelteKit"},
		{Key: "angular", Label: "Angular"},
		{Key: "express", Label: "Express / Fastify / Koa (backend, no build step)"},
		{Key: "plain", Label: "Plain Node.js"},
	}
	return prompt.Select("Select your framework:", choices, "plain")
}

func askNotify() string {
	choices := []prompt.Choice{
		{Key: "none", Label: "None"},
		{Key: "slack", Label: "Slack (needs SLACK_WEBHOOK_URL secret)"},
		{Key: "discord", Label: "Discord (needs DISCORD_WEBHOOK_URL secret)"},
		{Key: "email", Label: "Email (needs MAIL_USERNAME + MAIL_PASSWORD secrets)"},
	}
	return prompt.Select("Send CI notifications to?", choices, "none")
}

func printSummary(ctx config.ProjectContext) {
	fmt.Println("\n Detection results:")
	fmt.Printf("   Language     : %s %s\n", ctx.Language, ctx.Runtime)
	if ctx.Framework != "" {
		fmt.Printf("   Framework    : %s\n", ctx.Framework)
	}
	if ctx.BuildCommand != "" {
		fmt.Printf("   Build cmd    : %s\n", ctx.BuildCommand)
	}
	fmt.Printf("   Docker       : %v (compose: %v)\n", ctx.HasDocker, ctx.HasCompose)
	fmt.Printf("   Test command : %s\n", ctx.TestCommand)
	fmt.Printf("   Cloud        : %s\n", ctx.Cloud)
	fmt.Printf("   Main branch  : %s\n", ctx.MainBranch)
	fmt.Printf("   Repo name    : %s\n", ctx.RepoName)
}

func printReport(ctx config.ProjectContext, outPath string) {
	fmt.Println("\n" + repeat("─", 52))
	fmt.Println("  Pipeline generated successfully!")
	fmt.Println(repeat("─", 52))

	fmt.Printf("\n   File       : %s\n", outPath)
	fmt.Printf("   Language   : %s %s\n", ctx.Language, ctx.Runtime)
	if ctx.Framework != "" && ctx.Framework != "plain" {
		fmt.Printf("  🧩 Framework  : %s\n", ctx.Framework)
	}
	if ctx.BuildCommand != "" {
		fmt.Printf("  🔨 Build      : %s\n", ctx.BuildCommand)
	}
	if ctx.TestCommand != "" {
		fmt.Printf("  🧪 Tests      : %s\n", ctx.TestCommand)
	}
	if ctx.HasDocker {
		fmt.Printf("  🐳 Registry   : %s\n", ctx.Registry)
	}
	if ctx.Notify != "none" && ctx.Notify != "" {
		fmt.Printf("  🔔 Notify     : %s\n", ctx.Notify)
	}
	fmt.Printf("  🌿 Branch     : %s\n", ctx.MainBranch)

	// Required secrets
	secrets := requiredSecrets(ctx)
	if len(secrets) > 0 {
		fmt.Println("\n  🔑 Required secrets (GitHub → Settings → Secrets):")
		for _, s := range secrets {
			fmt.Printf("     • %s\n", s)
		}
	}

	fmt.Println("\n  Next steps:")
	fmt.Println("    1. Review the generated file")
	if len(secrets) > 0 {
		fmt.Println("    2. Add the required secrets above in GitHub")
		fmt.Println("    3. git add .github && git commit -m 'ci: add deploya pipeline'")
		fmt.Println("    4. Push and watch your pipeline run 🚀")
	} else {
		fmt.Println("    2. git add .github && git commit -m 'ci: add deploya pipeline'")
		fmt.Println("    3. Push and watch your pipeline run 🚀")
	}
	fmt.Println()
}

func requiredSecrets(ctx config.ProjectContext) []string {
	var secrets []string
	switch ctx.Registry {
	case "dockerhub":
		secrets = append(secrets, "DOCKERHUB_USERNAME", "DOCKERHUB_TOKEN")
	case "ecr":
		secrets = append(secrets, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION")
	case "gcr":
		secrets = append(secrets, "GCP_SA_KEY", "GCP_PROJECT_ID")
	}
	switch ctx.Notify {
	case "slack":
		secrets = append(secrets, "SLACK_WEBHOOK_URL")
	case "discord":
		secrets = append(secrets, "DISCORD_WEBHOOK_URL")
	case "email":
		secrets = append(secrets, "MAIL_USERNAME", "MAIL_PASSWORD", "MAIL_TO")
	}
	return secrets
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
