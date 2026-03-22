package cmd

import (
	"flag"
	"fmt"

	"github.com/kushalsubedi/deploya/config"
	"github.com/kushalsubedi/deploya/detector"
	"github.com/kushalsubedi/deploya/generator"
	"github.com/kushalsubedi/deploya/prompt"
	"github.com/kushalsubedi/deploya/releaserc"
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
	fmt.Println("🔍 Scanning project...")
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

	// ── Release pipeline ───────────────────────────────────────────
	var relCfg *releaserc.Config
	relCfg = handleRelease(*dir, ctx)

	// ── Generate CI pipeline ───────────────────────────────────────
	fmt.Println("\n⚙️  Generating GitHub Actions pipeline...")
	content, err := generator.Generate(ctx, *dir)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	outPath, err := generator.Write(content, *dir)
	if err != nil {
		return fmt.Errorf("failed to write pipeline: %w", err)
	}

	// ── Generate release pipeline if releaserc configured ──────────
	var releasePipelinePath string
	if relCfg != nil {
		releasePipelinePath, err = generator.WriteReleasePipeline(*relCfg, *dir)
		if err != nil {
			fmt.Printf("  ⚠️  Warning: could not write release pipeline: %v\n", err)
		}
	}

	// ── Generate GitHub templates ──────────────────────────────────
	fmt.Println("📝 Generating GitHub templates...")
	written, err := generator.WriteGithubTemplates(*dir)
	if err != nil {
		fmt.Printf("  ⚠️  Warning: could not write GitHub templates: %v\n", err)
	} else {
		for _, f := range written {
			fmt.Printf("   ✅ %s\n", f)
		}
	}

	// ── Summary report ─────────────────────────────────────────────
	printReport(ctx, outPath, releasePipelinePath)
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
	fmt.Println("\n📋 Detection results:")
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

// handleRelease checks for .releaserc or asks user if they want releases.
// Returns a config pointer if release pipeline should be generated, nil otherwise.
func handleRelease(dir string, ctx config.ProjectContext) *releaserc.Config {
	// Check if .releaserc already exists
	if cfg, err := releaserc.Load(dir); err == nil {
		fmt.Println("\n📦 Found .releaserc — release pipeline will be generated")
		return &cfg
	}

	// Ask user if they want automated releases
	if !prompt.Confirm("Do you want to set up automated releases? (deploya release)", false) {
		return nil
	}

	// Ask for github_repo
	fmt.Print("\n  Enter your GitHub repo (owner/repo): ")
	var repo string
	fmt.Scanln(&repo)
	if repo == "" || !containsSlash(repo) {
		fmt.Println("  ⚠️  Skipping release setup — invalid repo format")
		return nil
	}

	// Ask archive
	archive := prompt.Confirm("Build release binaries for all platforms?", true)

	// Build config with defaults + user input
	cfg := releaserc.DefaultConfig()
	cfg.GithubRepo = repo
	cfg.Archive = archive
	cfg.Registry = ctx.Registry
	cfg.OnBranch = ctx.MainBranch
	cfg.CurrentVersion = "v0.0.0"

	// Save .releaserc to project
	if err := releaserc.Save(dir, cfg); err != nil {
		fmt.Printf("  ⚠️  Could not save .releaserc: %v\n", err)
		return nil
	}

	fmt.Println("  ✅ Created .releaserc")
	return &cfg
}

func containsSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}

func printReport(ctx config.ProjectContext, outPath, releasePath string) {
	fmt.Println("\n" + repeat("─", 52))
	fmt.Println("  ✅ Pipeline generated successfully!")
	fmt.Println(repeat("─", 52))

	fmt.Printf("\n  📄 CI pipeline   : %s\n", outPath)
	if releasePath != "" {
		fmt.Printf("  📄 Release pipeline: %s\n", releasePath)
	}
	fmt.Printf("  🔤 Language   : %s %s\n", ctx.Language, ctx.Runtime)
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
	fmt.Println("    1. Review the generated file(s)")
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
