package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kushalsubedi/deploya/releaserc"
)

// releaseOpts drives a single repo's release. Shared by `deploya release`
// and `deploya org release` so both paths behave identically.
type releaseOpts struct {
	Dir    string
	Only   []string // package names
	Group  string   // package group name
	DryRun bool
	Token  string
}

// releaseResult reports what a repo release did, for org-level summaries.
type releaseResult struct {
	Released []string // tags created
	Skipped  int
}

func runRelease(args []string) error {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory")
	dryRun := fs.Bool("dry-run", false, "Preview release without publishing")
	only := fs.String("only", "", "Comma-separated package names to release (monorepo)")
	group := fs.String("group", "", "Release a configured package group (monorepo)")
	fs.Usage = func() {
		fmt.Println(`Usage: deploya release [flags]

Reads .releaserc, bumps version(s), generates changelog(s) and publishes
GitHub release(s). In a monorepo each package in .releaserc is versioned from
the commits that touched its path.

Flags:`)
		fs.PrintDefaults()
		fmt.Println(`
Examples:
  deploya release                     # every package with changes
  deploya release --only api,worker   # just these packages
  deploya release --group platform    # a configured group
  deploya release --dry-run`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, err := releaseRepo(releaseOpts{
		Dir:    *dir,
		Only:   splitList(*only),
		Group:  *group,
		DryRun: *dryRun,
		Token:  githubToken(),
	})
	return err
}

// releaseRepo plans and executes every release for one repository.
func releaseRepo(o releaseOpts) (releaseResult, error) {
	var result releaseResult

	// ── Load .releaserc ────────────────────────────────────────────
	fmt.Println("📦 Loading .releaserc...")
	cfg, err := releaserc.Load(o.Dir)
	if err != nil {
		return result, fmt.Errorf("could not load .releaserc: %w\nRun 'deploya init' first", err)
	}
	fmt.Printf("   Repo         : %s\n", cfg.GithubRepo)
	fmt.Printf("   Branch       : %s\n", cfg.OnBranch)
	if cfg.IsMonorepo() {
		fmt.Printf("   Mode         : %s (%d packages)\n", cfg.Mode, len(cfg.Packages))
	} else {
		fmt.Printf("   Current ver  : %s\n", cfg.CurrentVersion)
	}

	// ── Token: required to publish, optional for a dry run ─────────
	if o.Token == "" && !o.DryRun {
		return result, fmt.Errorf("no GitHub token found\nSet GH_TOKEN (or GITHUB_TOKEN) with: export GH_TOKEN=your_token")
	}

	// ── Plan ───────────────────────────────────────────────────────
	fmt.Println("\n🔍 Planning release...")
	plan, err := releaserc.BuildPlan(o.Dir, cfg, o.Only, o.Group)
	if err != nil {
		return result, err
	}
	printPlan(plan)

	result.Skipped = len(plan.Skipped)
	if len(plan.Releases) == 0 {
		fmt.Println("\n⚠️  Nothing to release.")
		return result, nil
	}

	var gh *releaserc.GitHub
	if o.Token != "" {
		gh = releaserc.NewGitHub(o.Token, cfg.GithubRepo)
	}

	// ── Execute, unit by unit ──────────────────────────────────────
	var changelogs []string
	for _, rel := range plan.Releases {
		changelog, err := executeRelease(o, cfg, gh, rel)
		if err != nil {
			// Earlier units already published; say so rather than
			// pretending the whole release rolled back.
			if len(result.Released) > 0 {
				fmt.Printf("\n⚠️  Already released: %s\n", strings.Join(result.Released, ", "))
			}
			return result, fmt.Errorf("release of %s failed: %w", rel.Pkg.Label(), err)
		}
		if o.DryRun {
			continue
		}
		result.Released = append(result.Released, rel.Tag)
		cfg.SetVersion(rel.Pkg.Name, rel.Version)
		if changelog != "" {
			changelogs = append(changelogs, changelog)
		}
	}

	if o.DryRun {
		fmt.Println("\n🔍 Dry run — nothing published.")
		return result, nil
	}

	// ── Persist versions and changelogs in one commit ──────────────
	fmt.Println("\n💾 Updating .releaserc...")
	if err := releaserc.Save(o.Dir, cfg); err != nil {
		fmt.Printf("   ⚠️  Could not update .releaserc: %v\n", err)
	} else {
		fmt.Println("   ✅ versions updated")
	}

	fmt.Println("\n📤 Committing release files...")
	files := append([]string{".releaserc"}, changelogs...)
	msg := fmt.Sprintf("chore: release %s [skip ci]", strings.Join(result.Released, ", "))
	if err := releaserc.CommitAndPush(o.Dir, msg, files); err != nil {
		fmt.Printf("   ⚠️  Could not commit release files: %v\n", err)
	} else {
		fmt.Println("   ✅ Committed and pushed")
	}

	printReleaseSummary(cfg, plan, result)
	return result, nil
}

// executeRelease publishes one unit. Returns the changelog path to commit,
// relative to the repo root.
func executeRelease(o releaseOpts, cfg releaserc.Config, gh *releaserc.GitHub, rel releaserc.Release) (string, error) {
	unitDir := rel.Pkg.Dir(o.Dir)

	fmt.Println("\n" + repeat("━", 52))
	fmt.Printf("  %s  %s → %s  (%s bump, %s)\n",
		rel.Pkg.Label(), displayPrev(rel.PrevTag), rel.Tag, rel.Bump, rel.Reason)
	fmt.Println(repeat("━", 52))

	// ── Enrich commits with GitHub PR data ─────────────────────────
	commits := rel.Commits
	if gh != nil && len(commits) > 0 {
		fmt.Println("🔗 Fetching PR data from GitHub...")
		enriched, err := gh.EnrichCommits(commits)
		if err != nil {
			fmt.Printf("   ⚠️  Could not fetch PR data: %v\n", err)
			fmt.Println("   Continuing with commit messages only...")
		} else {
			commits = enriched
			fmt.Printf("   PRs matched  : %d\n", countPRs(commits))
		}
	}

	// ── Build release assets (binaries + container image) ──────────
	// Built before the notes so the changelog links exactly what shipped.
	assets := &releaserc.Assets{}
	artifactName := rel.Pkg.ArtifactName(cfg.GithubRepo)

	if rel.Pkg.Archive {
		if o.DryRun {
			assets.Files = releaserc.PlanArchives(artifactName, rel.Version)
			fmt.Printf("\n📦 Would build %d binary archives (dry run)\n", len(assets.Files))
		} else {
			fmt.Println("\n📦 Building binary archives...")
			files, err := releaserc.BuildArchives(unitDir, artifactName, rel.Version)
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

	if ref := releaserc.PlanImageRef(unitDir, cfg, rel.Pkg, rel.Version); ref != "" {
		if o.DryRun {
			assets.ImageRef = ref
			fmt.Printf("\n🐳 Would push container image %s (dry run)\n", ref)
		} else {
			fmt.Printf("\n🐳 Building and pushing container image %s...\n", ref)
			pushed, err := releaserc.BuildAndPushImage(unitDir, cfg, rel.Pkg, rel.Version, o.Token)
			if err != nil {
				return "", fmt.Errorf("could not push container image %s: %w", ref, err)
			}
			assets.ImageRef = pushed
			fmt.Printf("   ✅ Pushed %s (and :latest)\n", pushed)
		}
	}

	// ── Release notes ──────────────────────────────────────────────
	categories := releaserc.CategorizeCommits(commits, cfg.Categories)
	notes := releaserc.GenerateNotes(rel, categories, cfg.GithubRepo, assets)

	fmt.Println("\n📝 Release notes preview:")
	fmt.Println(repeat("─", 52))
	fmt.Println(notes)
	fmt.Println(repeat("─", 52))

	if o.DryRun {
		return "", nil
	}

	// ── Changelog lives next to the code it describes ──────────────
	fmt.Println("📄 Updating CHANGELOG.md...")
	changelog := filepath.Join(rel.Pkg.Path, "CHANGELOG.md")
	if err := releaserc.UpdateChangelog(unitDir, notes); err != nil {
		fmt.Printf("   ⚠️  Could not update CHANGELOG.md: %v\n", err)
		changelog = ""
	} else {
		fmt.Printf("   ✅ %s updated\n", changelog)
	}

	// ── Tag first so the release is anchored to real branch history ─
	fmt.Printf("\n🏷️  Creating tag %s...\n", rel.Tag)
	if err := releaserc.CreateSignedTag(o.Dir, rel.Tag, fmt.Sprintf("Release %s", rel.Tag)); err != nil {
		return "", fmt.Errorf("could not create tag: %w", err)
	}

	title := rel.Tag
	if rel.Pkg.Name != "" {
		title = rel.Pkg.Name + " " + rel.Version
	}
	fmt.Printf("\n🚀 Creating GitHub release %s...\n", title)
	releaseID, err := gh.CreateRelease(title, rel.Tag, notes, cfg.OnBranch)
	if err != nil {
		return "", fmt.Errorf("could not create GitHub release: %w", err)
	}
	fmt.Printf("   ✅ https://github.com/%s/releases/tag/%s\n", cfg.GithubRepo, rel.Tag)

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

	return changelog, nil
}

func printPlan(plan releaserc.Plan) {
	if len(plan.Releases) > 0 {
		fmt.Printf("\n   Will release (%s mode):\n", plan.Mode)
		for _, r := range plan.Releases {
			fmt.Printf("     • %-14s %s → %s  (%s, %d commits, %s)\n",
				r.Pkg.Label(), displayPrev(r.PrevTag), r.Tag, r.Bump, len(r.Commits), r.Reason)
		}
	}
	if len(plan.Skipped) > 0 {
		fmt.Println("\n   Skipped:")
		for _, s := range plan.Skipped {
			fmt.Printf("     · %-14s %s\n", s.Name, s.Reason)
		}
	}
}

func printReleaseSummary(cfg releaserc.Config, plan releaserc.Plan, result releaseResult) {
	fmt.Println("\n" + repeat("─", 52))
	fmt.Printf("  🎉 Released %d unit(s) successfully!\n", len(result.Released))
	fmt.Println(repeat("─", 52))
	for _, r := range plan.Releases {
		fmt.Printf("\n  📦 %s\n", r.Pkg.Label())
		fmt.Printf("     Tag       : %s\n", r.Tag)
		fmt.Printf("     Release   : https://github.com/%s/releases/tag/%s\n", cfg.GithubRepo, r.Tag)
	}
	if result.Skipped > 0 {
		fmt.Printf("\n  · %d unit(s) skipped — no changes\n", result.Skipped)
	}
	fmt.Println()
}

func displayPrev(tag string) string {
	if tag == "" {
		return "(first)"
	}
	return tag
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

// githubToken reads the release token from the usual env vars.
func githubToken() string {
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GITHUB_TOKEN")
}

// splitList parses a comma-separated flag value.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
