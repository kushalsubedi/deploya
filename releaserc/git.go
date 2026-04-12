package releaserc

import (
	"fmt"
	"os"
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

// LatestTag returns the most recent tag that is reachable from HEAD.
// Falls back to version sort if no reachable tags exist.
func LatestTag(dir string) (string, error) {
	if err := checkGit(); err != nil {
		return "", err
	}

	// Fetch remote tags first
	_ = runGit(dir, "fetch", "--tags", "--quiet")

	// Try reachable tags first — these are properly anchored to branch history
	out, err := runGitOutput(dir, "describe", "--tags", "--abbrev=0")
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}

	// Fall back to version sort — handles orphan tags created via GitHub API
	out, err = runGitOutput(dir, "tag", "--sort=-version:refname")
	if err != nil || strings.TrimSpace(out) == "" {
		return "", nil
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[0]), nil
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
	// Configure git user if not already set (e.g. in CI)
	if name, _ := runGitOutput(dir, "config", "user.name"); strings.TrimSpace(name) == "" {
		_ = runGit(dir, "config", "user.name", "github-actions[bot]")
		_ = runGit(dir, "config", "user.email", "github-actions[bot]@users.noreply.github.com")
	}

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

// CreateSignedTag creates a GPG signed tag and pushes it.
// Falls back to unsigned tag if GPG is not configured.
func CreateSignedTag(dir, tag, message string) error {
	if err := checkGit(); err != nil {
		return err
	}

	// Check if GPG signing is configured
	signingKey, _ := runGitOutput(dir, "config", "user.signingkey")
	gpgSign, _ := runGitOutput(dir, "config", "tag.gpgSign")
	hasGPG := strings.TrimSpace(signingKey) != "" || strings.TrimSpace(gpgSign) == "true"

	if hasGPG {
		fmt.Println("   🔐 GPG key found — creating signed tag")
		if err := runGit(dir, "tag", "-s", tag, "-m", message); err != nil {
			fmt.Printf("   ⚠️  Signed tag failed, falling back to unsigned: %v\n", err)
			// fallback to unsigned
			if err := runGit(dir, "tag", "-a", tag, "-m", message); err != nil {
				return fmt.Errorf("tag creation failed: %w", err)
			}
		}
	} else {
		fmt.Println("   ℹ️  No GPG key configured — creating unsigned tag")
		if err := runGit(dir, "tag", "-a", tag, "-m", message); err != nil {
			return fmt.Errorf("tag creation failed: %w", err)
		}
	}

	// Push tag
	if err := runGit(dir, "push", "origin", tag); err != nil {
		return fmt.Errorf("tag push failed: %w", err)
	}

	fmt.Printf("   ✅ Tag %s pushed\n", tag)
	return nil
}

// ImportGPGKey imports a GPG private key from a base64 encoded string.
// Used in CI where the key is stored as a GitHub secret.
func ImportGPGKey(key string) error {
	// Write key to temp file
	tmpFile, err := os.CreateTemp("", "gpg-key-*.asc")
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(key); err != nil {
		return fmt.Errorf("could not write GPG key: %w", err)
	}
	tmpFile.Close()

	// Import the key
	cmd := exec.Command("gpg", "--batch", "--import", tmpFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gpg import failed: %s", string(out))
	}

	// Get key ID and configure git to use it
	listOut, err := exec.Command("gpg", "--list-secret-keys", "--keyid-format=long").Output()
	if err != nil {
		return fmt.Errorf("could not list GPG keys: %w", err)
	}

	// Parse key ID from output
	keyID := parseGPGKeyID(string(listOut))
	if keyID == "" {
		return fmt.Errorf("could not parse GPG key ID from output")
	}

	// Configure git to use this key
	if err := exec.Command("git", "config", "--global", "user.signingkey", keyID).Run(); err != nil {
		return fmt.Errorf("could not configure git signing key: %w", err)
	}
	if err := exec.Command("git", "config", "--global", "tag.gpgSign", "true").Run(); err != nil {
		return fmt.Errorf("could not enable git tag signing: %w", err)
	}
	if err := exec.Command("git", "config", "--global", "gpg.program", "gpg").Run(); err != nil {
		return fmt.Errorf("could not set gpg program: %w", err)
	}

	return nil
}

func parseGPGKeyID(output string) string {
	// Look for line like: sec   rsa4096/ABCDEF1234567890
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "sec") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Format: rsa4096/KEYID
				keyParts := strings.Split(parts[1], "/")
				if len(keyParts) == 2 {
					return keyParts[1]
				}
			}
		}
	}
	return ""
}

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
