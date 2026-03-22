package releaserc

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitLog reads all commits between fromTag and HEAD.
// Returns a slice of CommitInfo with SHA and raw message filled in.
// Title, PRNumber and Label are filled later by the GitHub API enricher.
func GitLog(dir, fromTag string) ([]CommitInfo, error) {
	// Verify git is available
	if err := checkGit(); err != nil {
		return nil, err
	}

	// Verify the directory is a git repo
	if err := runGit(dir, "rev-parse", "--git-dir"); err != nil {
		return nil, fmt.Errorf("not a git repository: %s", dir)
	}

	// Build range — if no tag yet, get all commits
	var logRange string
	if fromTag == "" || fromTag == "v0.0.0" {
		logRange = "HEAD"
	} else {
		// Verify the tag exists
		if err := runGit(dir, "rev-parse", fromTag); err != nil {
			return nil, fmt.Errorf("tag %q not found in repository", fromTag)
		}
		logRange = fromTag + "..HEAD"
	}

	// Run git log — format: SHA<tab>subject
	out, err := runGitOutput(dir, "log", logRange, "--format=%H\t%s", "--no-merges")
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	if strings.TrimSpace(out) == "" {
		return []CommitInfo{}, nil
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		sha := strings.TrimSpace(parts[0])
		msg := strings.TrimSpace(parts[1])

		commits = append(commits, CommitInfo{
			SHA:     sha,
			Message: msg,
			Title:   msg, // will be overridden by GitHub API if PR found
		})
	}

	return commits, nil
}

// LatestTag returns the most recent git tag reachable from HEAD.
// Returns empty string if no tags exist.
func LatestTag(dir string) (string, error) {
	if err := checkGit(); err != nil {
		return "", err
	}

	out, err := runGitOutput(dir, "describe", "--tags", "--abbrev=0")
	if err != nil {
		// No tags yet — that's fine
		return "", nil
	}

	return strings.TrimSpace(out), nil
}

// CurrentSHA returns the SHA of HEAD.
func CurrentSHA(dir string) (string, error) {
	out, err := runGitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not get HEAD SHA: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// CommitAndPush stages the given files, commits and pushes them.
// Used after a release to commit updated .releaserc and CHANGELOG.md.
func CommitAndPush(dir, message string, files []string) error {
	// Stage files
	args := append([]string{"add"}, files...)
	if err := runGit(dir, args...); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Commit
	if err := runGit(dir, "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Get current branch name
	branch, err := runGitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("could not get current branch: %w", err)
	}
	branch = strings.TrimSpace(branch)

	// Push — set upstream automatically if not set
	if err := runGit(dir, "push", "--set-upstream", "origin", branch); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
}

// ── helpers ────────────────────────────────────────────────────────────────

func checkGit() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed or not in PATH")
	}
	return nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runGitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
