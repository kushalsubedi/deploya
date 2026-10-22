package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/kushalsubedi/deploya/releaserc"
)

const orgUsage = `Usage: deploya org <subcommand> [flags]

Release a portfolio of repositories from one manifest (` + releaserc.OrgManifestFile + `).
Each repo keeps its own .releaserc; the manifest only says which repos belong
to the product and which packages inside them to release.

Subcommands:
  init      Scaffold ` + releaserc.OrgManifestFile + ` in the current directory
  status    Show what is unreleased across every repo
  release   Cut releases across every repo, isolating failures per repo

Flags:
  --dir      Directory holding the manifest (default ".")
  --only     Comma-separated repos to act on (owner/repo or just repo)
  --dry-run  Preview without publishing (release only)

Examples:
  deploya org init
  deploya org status
  deploya org release --dry-run
  deploya org release --only api,web
`

func runOrg(args []string) error {
	if len(args) == 0 {
		fmt.Print(orgUsage)
		return nil
	}

	sub := args[0]
	fs := flag.NewFlagSet("org "+sub, flag.ExitOnError)
	dir := fs.String("dir", ".", "Directory holding the manifest")
	only := fs.String("only", "", "Comma-separated repos to act on")
	dryRun := fs.Bool("dry-run", false, "Preview without publishing")
	fs.Usage = func() { fmt.Print(orgUsage) }

	switch sub {
	case "init":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return orgInit(*dir)
	case "status":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return orgRun(*dir, splitList(*only), true, false)
	case "release":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return orgRun(*dir, splitList(*only), false, *dryRun)
	case "--help", "-h", "help":
		fmt.Print(orgUsage)
		return nil
	default:
		fmt.Print(orgUsage)
		return fmt.Errorf("unknown org subcommand: %q", sub)
	}
}

// orgInit scaffolds a manifest, seeding it with the current repo when one is
// already configured for releases.
func orgInit(dir string) error {
	if _, err := releaserc.LoadOrg(dir); err == nil {
		return fmt.Errorf("%s already exists", releaserc.OrgManifestFile)
	}

	m := releaserc.OrgManifest{Org: "your-org"}
	if cfg, err := releaserc.Load(dir); err == nil {
		owner := strings.SplitN(cfg.GithubRepo, "/", 2)[0]
		m.Org = owner
		m.Repos = append(m.Repos, releaserc.OrgRepo{Repo: cfg.GithubRepo, Dir: "."})
	}
	m.Repos = append(m.Repos, releaserc.OrgRepo{Repo: m.Org + "/another-repo"})

	if err := releaserc.SaveOrg(dir, m); err != nil {
		return err
	}
	fmt.Printf("✅ Created %s\n\n", releaserc.OrgManifestFile)
	fmt.Println("  Next steps:")
	fmt.Println("    1. List your repos under `repos:` (owner/repo each)")
	fmt.Println("    2. Point `dir:` at local checkouts, or let deploya clone them")
	fmt.Println("    3. deploya org status")
	return nil
}

// orgRun walks the manifest. statusOnly stops after planning; otherwise each
// repo is released. A failing repo never stops the others.
func orgRun(dir string, only []string, statusOnly, dryRun bool) error {
	m, err := releaserc.LoadOrg(dir)
	if err != nil {
		return err
	}
	repos, err := m.Select(only)
	if err != nil {
		return err
	}
	token := githubToken()
	if token == "" && !statusOnly && !dryRun {
		return fmt.Errorf("no GitHub token found\nSet GH_TOKEN (or GITHUB_TOKEN) with: export GH_TOKEN=your_token")
	}

	fmt.Printf("🏢 Org: %s — %d repo(s)\n", m.Org, len(repos))

	type outcome struct {
		repo     string
		released []string // tags published (release mode)
		pending  int      // units that would release (status mode)
		err      error
	}
	var results []outcome

	for _, r := range repos {
		fmt.Println("\n" + repeat("═", 52))
		fmt.Printf("  %s\n", r.Repo)
		fmt.Println(repeat("═", 52))

		repoDir, err := m.Checkout(dir, r, token)
		if err != nil {
			fmt.Printf("  ❌ %v\n", err)
			results = append(results, outcome{repo: r.Repo, err: err})
			continue
		}

		if statusOnly {
			pending, err := orgStatusOne(repoDir, r)
			if err != nil {
				fmt.Printf("  ❌ %v\n", err)
				results = append(results, outcome{repo: r.Repo, err: err})
				continue
			}
			results = append(results, outcome{repo: r.Repo, pending: pending})
			continue
		}

		res, err := releaseRepo(releaseOpts{
			Dir:    repoDir,
			Only:   r.Only,
			Group:  r.Group,
			DryRun: dryRun,
			Token:  token,
		})
		if err != nil {
			fmt.Printf("  ❌ %v\n", err)
		}
		results = append(results, outcome{repo: r.Repo, released: res.Released, err: err})
	}

	// ── Portfolio summary ──────────────────────────────────────────
	fmt.Println("\n" + repeat("─", 52))
	fmt.Println("  🏢 Org summary")
	fmt.Println(repeat("─", 52))
	failed := 0
	for _, o := range results {
		switch {
		case o.err != nil:
			failed++
			fmt.Printf("  ❌ %-30s %v\n", o.repo, o.err)
		case len(o.released) > 0:
			fmt.Printf("  ✅ %-30s %s\n", o.repo, strings.Join(o.released, ", "))
		case o.pending > 0:
			fmt.Printf("  ⬆️  %-30s %d unit(s) pending release\n", o.repo, o.pending)
		default:
			fmt.Printf("  · %-30s no changes\n", o.repo)
		}
	}
	fmt.Println()

	if failed > 0 {
		return fmt.Errorf("%d of %d repo(s) failed", failed, len(results))
	}
	return nil
}

// orgStatusOne prints the release plan for a single repo without publishing
// and reports how many units are waiting to go out.
func orgStatusOne(repoDir string, r releaserc.OrgRepo) (int, error) {
	cfg, err := releaserc.Load(repoDir)
	if err != nil {
		return 0, err
	}
	plan, err := releaserc.BuildPlan(repoDir, cfg, r.Only, r.Group)
	if err != nil {
		return 0, err
	}
	if len(plan.Releases) == 0 && len(plan.Skipped) == 0 {
		fmt.Println("  · nothing configured")
		return 0, nil
	}
	printPlan(plan)
	return len(plan.Releases), nil
}
