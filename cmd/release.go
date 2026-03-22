package cmd

import (
	"flag"
	"fmt"
	"os"

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
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is not set\nSet it with: export GITHUB_TOKEN=your_token")
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
	fmt.Printf("\n🚀 Creating GitHub release %s...\n", nextVer.String())
	releaseID, err := gh.CreateRelease(nextVer.String(), cfg.TagPrefix+nextVer.String(), notes)
	if err != nil {
		return fmt.Errorf("could not create GitHub release: %w", err)
	}
	fmt.Printf("   ✅ Release created: https://github.com/%s/releases/tag/%s\n", cfg.GithubRepo, nextVer.String())

	// ── Build and upload archives ──────────────────────────────────
	if cfg.Archive {
		fmt.Println("\n📦 Building release archives...")
		archives, err := releaserc.BuildArchives(*dir, cfg.GithubRepo, nextVer.String())
		if err != nil {
			fmt.Printf("   ⚠️  Archive build failed: %v\n", err)
		} else {
			fmt.Printf("   Built %d archives — uploading...\n", len(archives))
			for _, a := range archives {
				if err := gh.UploadAsset(releaseID, a); err != nil {
					fmt.Printf("   ⚠️  Upload failed for %s: %v\n", a, err)
				} else {
					fmt.Printf("   ✅ %s\n", a)
				}
			}
		}
	}

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
	fmt.Printf("  🔖 Tag       : %s%s\n", cfg.TagPrefix, nextVer.String())
	fmt.Printf("  📝 Changelog : CHANGELOG.md\n")
	fmt.Printf("  🔗 Release   : https://github.com/%s/releases/tag/%s%s\n",
		cfg.GithubRepo, cfg.TagPrefix, nextVer.String())
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
