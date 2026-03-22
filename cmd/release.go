package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kushalsubedi/deploya/releaserc"
)

func runRelease(args []string) error {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory")
	dryRun := fs.Bool("dry-run", false, "Preview release without publishing")
	fs.Usage = func() {
		fmt.Println(`Usage: deploya release [flags]

Reads .releaserc, bumps version, generates changelog and publishes a GitHub releaserc.

Flags:`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// ── Load .releaserc ────────────────────────────────────────────
	fmt.Println("📦 Loading .releaserc...")
	cfg, err := releaserc.Load(*dir)
	if err != nil {
		return fmt.Errorf("could not load .releaserc: %w\nRun 'deploya init' first", err)
	}
	fmt.Printf("   Repo         : %s\n", cfg.GithubRepo)
	fmt.Printf("   Current ver  : %s\n", cfg.CurrentVersion)
	fmt.Printf("   Branch       : %s\n", cfg.OnBranch)
	fmt.Printf("   Archive      : %v\n", cfg.Archive)

	// ── Check GITHUB_TOKEN ─────────────────────────────────────────
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		return fmt.Errorf("GH_TOKEN environment variable is not set\nSet it with: export GH_TOKEN=your_token")
	}

	// ── Read git log ───────────────────────────────────────────────
	fmt.Println("\n🔍 Reading git history...")
	latestTag, err := releaserc.LatestTag(*dir)
	if err != nil {
		return fmt.Errorf("could not read git tags: %w", err)
	}

	fromTag := latestTag
	if fromTag == "" {
		fmt.Println("   No tags found — this will be the first release")
	} else {
		fmt.Printf("   Last tag     : %s\n", fromTag)
	}

	commits, err := releaserc.GitLog(*dir, fromTag)
	if err != nil {
		return fmt.Errorf("could not read git log: %w", err)
	}

	if len(commits) == 0 {
		fmt.Println("\n⚠️  No commits since last releaserc. Nothing to releaserc.")
		return nil
	}
	fmt.Printf("   Commits found: %d\n", len(commits))

	// ── Enrich commits with GitHub PR data ─────────────────────────
	fmt.Println("\n🔗 Fetching PR data from GitHub...")
	gh := releaserc.NewGitHub(token, cfg.GithubRepo)
	commits, err = gh.EnrichCommits(commits)
	if err != nil {
		fmt.Printf("   ⚠️  Could not fetch PR data: %v\n", err)
		fmt.Println("   Continuing with commit messages only...")
	} else {
		fmt.Printf("   PRs matched  : %d\n", countPRs(commits))
	}

	// ── Determine version bump ─────────────────────────────────────
	bump := releaserc.DetermineBump(commits)
	currentVer, err := releaserc.ParseVersion(cfg.CurrentVersion)
	if err != nil {
		return fmt.Errorf("invalid current_version in .releaserc: %w", err)
	}
	nextVer := currentVer.Bump(bump)

	fmt.Printf("\n📈 Version bump : %s\n", bump)
	fmt.Printf("   %s → %s\n", currentVer.String(), nextVer.String())

	// ── Categorize commits ─────────────────────────────────────────
	categories := releaserc.CategorizeCommits(commits, cfg.Categories)

	// ── Generate release notes ─────────────────────────────────────
	notes := releaserc.GenerateNotes(nextVer.String(), categories, currentVer.String(), cfg.GithubRepo)

	fmt.Println("\n📝 Release notes preview:")
	fmt.Println(repeat("─", 52))
	fmt.Println(notes)
	fmt.Println(repeat("─", 52))

	if *dryRun {
		fmt.Println("\n🔍 Dry run — nothing published.")
		return nil
	}

	// ── Update CHANGELOG.md ────────────────────────────────────────
	fmt.Println("\n📄 Updating CHANGELOG.md...")
	if err := releaserc.UpdateChangelog(*dir, notes); err != nil {
		fmt.Printf("   ⚠️  Could not update CHANGELOG.md: %v\n", err)
	} else {
		fmt.Println("   ✅ CHANGELOG.md updated")
	}

	// ── Create GitHub release ──────────────────────────────────────
	// Strip version prefix before concatenating with tag_prefix
	// e.g. tag_prefix="v" + version="v0.1.0" → strip to "0.1.0" → "v0.1.0"
	versionNoPrefix := strings.TrimPrefix(nextVer.String(), cfg.TagPrefix)
	tag := cfg.TagPrefix + versionNoPrefix

	fmt.Printf("\n🚀 Creating GitHub release %s...\n", nextVer.String())
	_, err = gh.CreateRelease(nextVer.String(), tag, notes)
	if err != nil {
		return fmt.Errorf("could not create GitHub release: %w", err)
	}
	fmt.Printf("   ✅ Release created: https://github.com/%s/releases/tag/%s\n", cfg.GithubRepo, tag)

	// ── Update .releaserc with new version ─────────────────────────
	fmt.Println("\n💾 Updating .releaserc...")
	cfg.CurrentVersion = nextVer.String()
	if err := releaserc.Save(*dir, cfg); err != nil {
		fmt.Printf("   ⚠️  Could not update .releaserc: %v\n", err)
	} else {
		fmt.Println("   ✅ current_version updated to", nextVer.String())
	}

	// ── Commit and push .releaserc + CHANGELOG.md ──────────────────
	fmt.Println("\n📤 Committing release files...")
	filesToCommit := []string{".releaserc", "CHANGELOG.md"}
	msg := fmt.Sprintf("chore: release %s [skip ci]", nextVer.String())
	if err := releaserc.CommitAndPush(*dir, msg, filesToCommit); err != nil {
		fmt.Printf("   ⚠️  Could not commit release files: %v\n", err)
	} else {
		fmt.Println("   ✅ Committed and pushed")
	}

	// ── Final summary ──────────────────────────────────────────────
	fmt.Println("\n" + repeat("─", 52))
	fmt.Printf("  🎉 Released %s successfully!\n", nextVer.String())
	fmt.Println(repeat("─", 52))
	fmt.Printf("\n  📦 Version   : %s\n", nextVer.String())
	fmt.Printf("  🔖 Tag       : %s\n", tag)
	fmt.Printf("  📝 Changelog : CHANGELOG.md\n")
	fmt.Printf("  🔗 Release   : https://github.com/%s/releases/tag/%s\n",
		cfg.GithubRepo, tag)
	fmt.Println()

	return nil
}

func countPRs(commits []releaserc.CommitInfo) int {
	count := 0
	for _, c := range commits {
		if c.PRNumber > 0 {
			count++
		}
	}
	return count
}
