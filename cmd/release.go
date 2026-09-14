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

Reads .releaserc, bumps version, generates changelog and publishes a GitHub release.

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

	// ── Check token (GH_TOKEN or GITHUB_TOKEN) ────────────────────
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("no GitHub token found\nSet GH_TOKEN (or GITHUB_TOKEN) with: export GH_TOKEN=your_token")
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
		fmt.Println("\n⚠️  No commits since last release. Nothing to release.")
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
	// The latest reachable git tag is the source of truth; .releaserc's
	// current_version is only a fallback for the first release.
	bump := releaserc.DetermineBump(commits)
	currentVer, err := releaserc.ParseVersion(cfg.CurrentVersion)
	if err != nil {
		return fmt.Errorf("invalid current_version in .releaserc: %w", err)
	}
	if latestTag != "" {
		bare := strings.TrimPrefix(latestTag, cfg.TagPrefix)
		if tagVer, tagErr := releaserc.ParseVersion(bare); tagErr == nil {
			if cfg.TagPrefix+trimVerPrefix(cfg.CurrentVersion, cfg.TagPrefix) != latestTag {
				fmt.Printf("   ⚠️  .releaserc says %s but latest tag is %s — using the tag\n", cfg.CurrentVersion, latestTag)
			}
			currentVer = tagVer
		}
	}
	nextVer := currentVer.Bump(bump)

	// ── Calculate tags up front so notes and links stay consistent ─
	tag := cfg.TagPrefix + trimVerPrefix(nextVer.String(), cfg.TagPrefix)
	prevTag := latestTag

	fmt.Printf("\n📈 Version bump : %s\n", bump)
	fmt.Printf("   %s → %s\n", currentVer.String(), tag)

	// ── Build release assets (binaries + container image) ──────────
	// Built before the notes so the changelog links exactly what shipped.
	assets := &releaserc.Assets{}

	if cfg.Archive {
		if *dryRun {
			assets.Files = releaserc.PlanArchives(cfg.GithubRepo, tag)
			fmt.Printf("\n📦 Would build %d binary archives (dry run)\n", len(assets.Files))
		} else {
			fmt.Println("\n📦 Building binary archives...")
			files, err := releaserc.BuildArchives(*dir, cfg.GithubRepo, tag)
			if err != nil {
				fmt.Printf("   ⚠️  Could not build archives: %v\n", err)
				fmt.Println("   Continuing without binary assets...")
			} else {
				assets.Files = files
				for _, f := range files {
					fmt.Printf("   ✅ %s\n", f.Name)
				}
			}
		}
	}

	if ref := releaserc.PlanImageRef(*dir, cfg, tag); ref != "" {
		if *dryRun {
			assets.ImageRef = ref
			fmt.Printf("\n🐳 Would push container image %s (dry run)\n", ref)
		} else {
			fmt.Printf("\n🐳 Building and pushing container image %s...\n", ref)
			pushed, err := releaserc.BuildAndPushImage(*dir, cfg, tag, token)
			if err != nil {
				fmt.Printf("   ⚠️  Could not push image: %v\n", err)
				fmt.Println("   Continuing without container asset...")
			} else {
				assets.ImageRef = pushed
				fmt.Printf("   ✅ Pushed %s (and :latest)\n", pushed)
			}
		}
	}

	// ── Categorize commits ─────────────────────────────────────────
	categories := releaserc.CategorizeCommits(commits, cfg.Categories)

	// ── Generate release notes ─────────────────────────────────────
	notes := releaserc.GenerateNotes(tag, categories, prevTag, cfg.GithubRepo, assets)

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

	// ── Create local tag and push ──────────────────────────────────
	// This anchors the tag to the real commit in branch history
	// so future git log ranges work correctly
	fmt.Printf("\n🏷️  Creating tag %s...\n", tag)
	if err := releaserc.CreateSignedTag(*dir, tag, fmt.Sprintf("Release %s", tag)); err != nil {
		return fmt.Errorf("could not create tag: %w", err)
	}

	// ── Create GitHub release from existing tag ────────────────────
	fmt.Printf("\n🚀 Creating GitHub release %s...\n", tag)
	releaseID, err := gh.CreateRelease(tag, tag, notes, cfg.OnBranch)
	if err != nil {
		return fmt.Errorf("could not create GitHub release: %w", err)
	}
	fmt.Printf("   ✅ Release created: https://github.com/%s/releases/tag/%s\n", cfg.GithubRepo, tag)

	// ── Upload binary archives to the release ──────────────────────
	if len(assets.Files) > 0 {
		fmt.Println("\n⬆️  Uploading release assets...")
		for _, f := range assets.Files {
			if f.Path == "" {
				continue
			}
			if err := gh.UploadAsset(releaseID, f.Path); err != nil {
				fmt.Printf("   ⚠️  Could not upload %s: %v\n", f.Name, err)
			} else {
				fmt.Printf("   ✅ %s\n", f.Name)
			}
		}
	}

	// ── Update .releaserc with new version ─────────────────────────
	fmt.Println("\n💾 Updating .releaserc...")
	cfg.CurrentVersion = tag
	if err := releaserc.Save(*dir, cfg); err != nil {
		fmt.Printf("   ⚠️  Could not update .releaserc: %v\n", err)
	} else {
		fmt.Println("   ✅ current_version updated to", tag)
	}

	// ── Commit and push .releaserc + CHANGELOG.md ──────────────────
	fmt.Println("\n📤 Committing release files...")
	filesToCommit := []string{".releaserc", "CHANGELOG.md"}
	msg := fmt.Sprintf("chore: release %s [skip ci]", tag)
	if err := releaserc.CommitAndPush(*dir, msg, filesToCommit); err != nil {
		fmt.Printf("   ⚠️  Could not commit release files: %v\n", err)
	} else {
		fmt.Println("   ✅ Committed and pushed")
	}

	// ── Final summary ──────────────────────────────────────────────
	fmt.Println("\n" + repeat("─", 52))
	fmt.Printf("  🎉 Released %s successfully!\n", tag)
	fmt.Println(repeat("─", 52))
	fmt.Printf("\n  📦 Version   : %s\n", tag)
	fmt.Printf("  🔖 Tag       : %s\n", tag)
	fmt.Printf("  📝 Changelog : CHANGELOG.md\n")
	if len(assets.Files) > 0 {
		fmt.Printf("  📦 Assets    : %d files\n", len(assets.Files))
	}
	if assets.ImageRef != "" {
		fmt.Printf("  🐳 Image     : %s\n", assets.ImageRef)
	}
	fmt.Printf("  🔗 Release   : https://github.com/%s/releases/tag/%s\n",
		cfg.GithubRepo, tag)
	fmt.Println()

	return nil
}

// trimVerPrefix strips the configured tag prefix (and a plain leading "v")
// so the prefix can be re-applied exactly once regardless of how
// current_version was written.
func trimVerPrefix(v, tagPrefix string) string {
	if tagPrefix != "" {
		v = strings.TrimPrefix(v, tagPrefix)
	}
	return strings.TrimPrefix(v, "v")
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
