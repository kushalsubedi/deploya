package cmd

import (
	"context"
	"flag"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kushalsubedi/deploya/pr"
	"github.com/kushalsubedi/deploya/prompt"
)

func runPR(args []string) error {
	fs := flag.NewFlagSet("pr", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory")
	baseFlag := fs.String("base", "", "Target base branch (default: auto-detect main/master)")
	headFlag := fs.String("head", "", "Source branch with unmerged commits (default: current branch)")
	repoFlag := fs.String("repo", "", "GitHub repository (owner/repo)")
	titleFlag := fs.String("title", "", "Pull Request title (overrides auto-generated title)")
	bodyFlag := fs.String("body", "", "Pull Request body (overrides auto-generated body)")
	draft := fs.Bool("draft", false, "Create Pull Request as draft")
	dryRun := fs.Bool("dry-run", false, "Preview PR title and description without creating it on GitHub")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	y := fs.Bool("y", false, "Alias for --yes")
	noAI := fs.Bool("no-ai", false, "Disable AI-generated PR summary")
	geminiKey := fs.String("gemini-key", "", "Google Gemini API key (defaults to GEMINI_API_KEY env)")
	autoPush := fs.Bool("push", true, "Automatically push branch to origin if unpushed")
	openWeb := fs.Bool("web", false, "Open created PR in default web browser")

	fs.Usage = func() {
		fmt.Println(`Usage: deploya pr [flags]

Create a GitHub Pull Request with unmerged commits and AI-generated summary.

Flags:`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	autoConfirm := *yes || *y

	// ── 1. Resolve Repository ───────────────────────────────────
	repo := *repoFlag
	if repo == "" {
		detectedRepo, err := pr.DetectRepo(*dir)
		if err != nil {
			return fmt.Errorf("could not detect repository: %w\nSpecify with --repo owner/repo", err)
		}
		repo = detectedRepo
	}

	// ── 2. Resolve Head Branch ──────────────────────────────────
	head := *headFlag
	if head == "" {
		detectedHead, err := pr.CurrentBranch(*dir)
		if err != nil {
			return fmt.Errorf("could not determine current branch: %w\nSpecify with --head <branch>", err)
		}
		head = detectedHead
	}

	// ── 3. Resolve Base Branch ──────────────────────────────────
	base := *baseFlag
	if base == "" {
		detectedBase, err := pr.DefaultBaseBranch(*dir)
		if err != nil {
			base = "main"
		} else {
			base = detectedBase
		}
	}

	if head == base {
		return fmt.Errorf("current branch %q is the same as base branch %q\nSwitch to a feature branch or specify --base / --head", head, base)
	}

	// ── 4. Collect Unmerged Commits ─────────────────────────────
	fmt.Printf("🔍 Repository   : %s\n", repo)
	fmt.Printf("🌿 Source (HEAD): %s\n", head)
	fmt.Printf("🎯 Target (BASE): %s\n", base)

	commits, err := pr.UnmergedCommits(*dir, base, head)
	if err != nil {
		return fmt.Errorf("could not retrieve unmerged commits: %w", err)
	}

	if len(commits) == 0 {
		fmt.Printf("\n⚠️  No unmerged commits found between %q and %q.\n", head, base)
		fmt.Println("   Everything is up to date! Nothing to create a PR for.")
		return nil
	}

	fmt.Printf("📦 Commits      : %d unmerged commit(s)\n", len(commits))
	for i, c := range commits {
		if i >= 5 {
			fmt.Printf("   ... and %d more commit(s)\n", len(commits)-5)
			break
		}
		fmt.Printf("   • %s %s\n", c.Short, c.Title)
	}

	// ── 5. Collect Diff Stats ───────────────────────────────────
	diffStat, _ := pr.DiffStat(*dir, base, head)
	diffPatch, _ := pr.DiffPatch(*dir, base, head, 30000)

	// ── 6. Generate PR Title & Description ──────────────────────
	var content pr.PRContent

	// Check if title or body was explicitly passed
	if *titleFlag != "" && *bodyFlag != "" {
		content.Title = *titleFlag
		content.Body = *bodyFlag
	} else {
		aiClient := pr.NewGeminiClient(*geminiKey)
		useAI := !*noAI && aiClient.IsAvailable()

		if useAI {
			fmt.Println("\n🤖 Generating beautiful PR with Gemini AI...")
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()

			aiContent, err := aiClient.GeneratePR(ctx, repo, head, base, commits, diffStat, diffPatch)
			if err != nil {
				fmt.Printf("   ⚠️  AI generation failed: %v\n", err)
				fmt.Println("   Falling back to built-in PR template...")
				content = pr.GenerateMarkdown(head, base, repo, commits, diffStat)
			} else {
				content = *aiContent
				fmt.Println("   ✨ AI generation completed!")
			}
		} else {
			fmt.Println("\n✨ Generating PR markdown from commit history...")
			if !*noAI && !aiClient.IsAvailable() {
				fmt.Println("   💡 Tip: Set GEMINI_API_KEY (free at https://aistudio.google.com) for AI-powered summaries!")
			}
			content = pr.GenerateMarkdown(head, base, repo, commits, diffStat)
		}

		if *titleFlag != "" {
			content.Title = *titleFlag
		}
		if *bodyFlag != "" {
			content.Body = *bodyFlag
		}
	}

	// ── 7. Preview PR ───────────────────────────────────────────
	sep := strings.Repeat("─", 54)
	fmt.Printf("\n%s\n", sep)
	fmt.Printf("📝 Pull Request Preview\n")
	fmt.Printf("%s\n", sep)
	fmt.Printf("Title: %s\n\n", content.Title)
	fmt.Println(content.Body)
	fmt.Printf("%s\n\n", sep)

	if *dryRun {
		fmt.Println("🔍 Dry run complete — PR was not submitted to GitHub.")
		return nil
	}

	// ── 8. Check Remote Push Status ─────────────────────────────
	pushed, _ := pr.IsBranchPushed(*dir, head)
	if !pushed {
		if *autoPush {
			fmt.Printf("🚀 Pushing branch %q to origin...\n", head)
			if err := pr.PushBranch(*dir, head); err != nil {
				return fmt.Errorf("could not push branch to remote: %w", err)
			}
			fmt.Println("   ✅ Branch pushed to origin")
		} else {
			fmt.Printf("⚠️  Branch %q has unpushed commits\n", head)
		}
	}

	// ── 9. Resolve GitHub Token ─────────────────────────────────
	token, err := pr.ResolveGitHubToken()
	if err != nil {
		return err
	}

	// ── 10. Confirmation ────────────────────────────────────────
	if !autoConfirm {
		if !prompt.Confirm("Ready to create pull request on GitHub?", true) {
			fmt.Println("\nCancelled. PR was not created.")
			return nil
		}
	}

	// ── 11. Create Pull Request on GitHub ───────────────────────
	fmt.Printf("\n🚀 Submitting Pull Request to %s...\n", repo)
	gh := pr.NewGitHubClient(token)
	res, err := gh.CreatePullRequest(repo, content.Title, head, base, content.Body, *draft)
	if err != nil {
		return err
	}

	fmt.Println("\n" + sep)
	fmt.Printf("🎉 Pull Request #%d created successfully!\n", res.Number)
	fmt.Println(sep)
	fmt.Printf("  🔗 PR URL  : %s\n", res.HTMLURL)
	fmt.Printf("  🌿 Head    : %s\n", res.Head)
	fmt.Printf("  🎯 Base    : %s\n", res.Base)
	if res.Draft {
		fmt.Println("  📝 Status  : Draft")
	}
	fmt.Println()

	// ── 12. Open in Browser if Requested ────────────────────────
	if *openWeb {
		_ = openBrowser(res.HTMLURL)
	}

	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
