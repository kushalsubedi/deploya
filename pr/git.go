package pr

import (
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"

	"github.com/kushalsubedi/deploya/releaserc"
)

// CurrentBranch returns the currently checked-out branch in dir.
func CurrentBranch(dir string) (string, error) {
	out, err := runGitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not get current branch: %w", err)
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("detached HEAD state, please checkout a named branch")
	}
	return branch, nil
}

// DefaultBaseBranch attempts to find the primary base branch (e.g. main or master).
func DefaultBaseBranch(dir string) (string, error) {
	// 1. Try loading from .releaserc if present
	if cfg, err := releaserc.Load(dir); err == nil && cfg.OnBranch != "" {
		return cfg.OnBranch, nil
	}

	// 2. Try git remote default branch: refs/remotes/origin/HEAD
	out, err := runGitOutput(dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(out)
		if idx := strings.LastIndex(ref, "/"); idx != -1 {
			return ref[idx+1:], nil
		}
	}

	// 3. Check if main branch exists locally or on remote
	if refExists(dir, "origin/main") || refExists(dir, "refs/heads/main") {
		return "main", nil
	}

	// 4. Check if master exists
	if refExists(dir, "origin/master") || refExists(dir, "refs/heads/master") {
		return "master", nil
	}

	return "main", nil
}

// DetectRepo resolves the GitHub "owner/repo" for the git repository in dir.
func DetectRepo(dir string) (string, error) {
	// 1. Check .releaserc first
	if cfg, err := releaserc.Load(dir); err == nil && cfg.GithubRepo != "" {
		return cfg.GithubRepo, nil
	}

	// 2. Check git remote origin URL
	out, err := runGitOutput(dir, "config", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("could not detect GitHub repository: no remote.origin.url found")
	}

	return ParseRepoFromURL(strings.TrimSpace(out))
}

// ParseRepoFromURL extracts "owner/repo" from various git remote URL formats.
func ParseRepoFromURL(remoteURL string) (string, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", fmt.Errorf("empty remote URL")
	}

	// Handle SCP-style: git@github.com:owner/repo.git
	if strings.HasPrefix(remoteURL, "git@") {
		colonIdx := strings.Index(remoteURL, ":")
		if colonIdx == -1 {
			return "", fmt.Errorf("invalid git SSH URL: %s", remoteURL)
		}
		path := remoteURL[colonIdx+1:]
		path = strings.TrimSuffix(path, ".git")
		path = strings.TrimPrefix(path, "/")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2] + "/" + parts[len(parts)-1], nil
		}
		return "", fmt.Errorf("cannot parse owner/repo from SSH URL: %s", remoteURL)
	}

	// Handle HTTPS / SSH URLs with scheme (https://github.com/owner/repo.git or ssh://git@...)
	parsed, err := url.Parse(remoteURL)
	if err == nil && parsed.Path != "" {
		path := strings.TrimPrefix(parsed.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2] + "/" + parts[len(parts)-1], nil
		}
	}

	return "", fmt.Errorf("unrecognized git remote URL format: %s", remoteURL)
}

var commitRegex = regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^)]+)\))?!?:\s*(.*)$`)

// UnmergedCommits returns all commits present in head that are not in base.
func UnmergedCommits(dir, base, head string) ([]Commit, error) {
	baseRef := resolveBaseRef(dir, base)
	logRange := fmt.Sprintf("%s..%s", baseRef, head)

	out, err := runGitOutput(dir, "log", logRange, "--format=%H%x09%s%x09%an", "--no-merges")
	if err != nil {
		return nil, fmt.Errorf("git log %s failed: %w", logRange, err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var commits []Commit
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		sha := parts[0]
		short := sha
		if len(short) > 7 {
			short = short[:7]
		}
		title := parts[1]
		author := ""
		if len(parts) >= 3 {
			author = parts[2]
		}

		c := Commit{
			SHA:     sha,
			Short:   short,
			Message: title,
			Title:   title,
			Author:  author,
			Type:    "other",
		}

		if matches := commitRegex.FindStringSubmatch(title); len(matches) == 4 {
			c.Type = strings.ToLower(matches[1])
			c.Scope = matches[2]
		}

		commits = append(commits, c)
	}

	return commits, nil
}

// DiffStat returns the summary of files changed between base and head.
func DiffStat(dir, base, head string) (string, error) {
	baseRef := resolveBaseRef(dir, base)
	out, err := runGitOutput(dir, "diff", "--stat", fmt.Sprintf("%s...%s", baseRef, head))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DiffPatch returns the diff between base and head, truncated to maxBytes.
func DiffPatch(dir, base, head string, maxBytes int) (string, error) {
	baseRef := resolveBaseRef(dir, base)
	out, err := runGitOutput(dir, "diff", fmt.Sprintf("%s...%s", baseRef, head))
	if err != nil {
		return "", err
	}
	if maxBytes > 0 && len(out) > maxBytes {
		return out[:maxBytes] + "\n\n... [diff truncated for brevity]", nil
	}
	return out, nil
}

// IsBranchPushed checks if the local branch HEAD matches origin/<head>.
func IsBranchPushed(dir, head string) (bool, error) {
	localSHA, err := runGitOutput(dir, "rev-parse", head)
	if err != nil {
		return false, err
	}
	localSHA = strings.TrimSpace(localSHA)

	remoteSHA, err := runGitOutput(dir, "rev-parse", "origin/"+head)
	if err != nil {
		// Remote branch probably doesn't exist yet
		return false, nil
	}
	remoteSHA = strings.TrimSpace(remoteSHA)

	return localSHA == remoteSHA, nil
}

// PushBranch pushes the branch to remote origin with -u.
func PushBranch(dir, head string) error {
	cmd := exec.Command("git", "push", "-u", "origin", head)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func resolveBaseRef(dir, base string) string {
	if refExists(dir, "origin/"+base) {
		return "origin/" + base
	}
	return base
}

func refExists(dir, ref string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", ref)
	cmd.Dir = dir
	return cmd.Run() == nil
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
