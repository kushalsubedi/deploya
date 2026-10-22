package releaserc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CategorizeCommits groups commits into categories based on labels and prefixes.
// Filters out deploya's own release commits ([skip ci]).
func CategorizeCommits(commits []CommitInfo, cats Categories) []Category {
	features := Category{Name: "Features", Emoji: "✨"}
	fixes := Category{Name: "Bug Fixes", Emoji: "🐛"}
	patches := Category{Name: "Patches & Maintenance", Emoji: "🔧"}
	docs := Category{Name: "Docs", Emoji: "📝"}

	for _, c := range commits {
		// Skip deploya's own release commits
		if isSkipCommit(c.Title) {
			continue
		}

		cat := categorizeOne(c, cats)
		switch cat {
		case "features":
			features.Commits = append(features.Commits, c)
		case "fixes":
			fixes.Commits = append(fixes.Commits, c)
		case "docs":
			docs.Commits = append(docs.Commits, c)
		default:
			patches.Commits = append(patches.Commits, c)
		}
	}

	// Only return non-empty categories
	var result []Category
	for _, cat := range []Category{features, fixes, patches, docs} {
		if len(cat.Commits) > 0 {
			result = append(result, cat)
		}
	}
	return result
}

// isSkipCommit returns true for commits that should not appear in changelog.
func isSkipCommit(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "[skip ci]") ||
		strings.Contains(lower, "chore: release") ||
		strings.HasPrefix(lower, "release ")
}

func categorizeOne(c CommitInfo, cats Categories) string {
	// Check label first
	if c.Label != "" {
		label := strings.ToLower(c.Label)
		if contains(cats.Features, label) || label == "enhancement" {
			return "features"
		}
		if contains(cats.Fixes, label) || label == "bug" {
			return "fixes"
		}
		if contains(cats.Docs, label) {
			return "docs"
		}
		if contains(cats.Patches, label) {
			return "patches"
		}
	}

	// Fall back to title prefix — hasPrefix also matches the "!" breaking
	// marker, so "feat!: x" is still a feature in the changelog.
	switch {
	case hasPrefix(c.Title, cats.Features...):
		return "features"
	case hasPrefix(c.Title, cats.Fixes...):
		return "fixes"
	case hasPrefix(c.Title, cats.Docs...):
		return "docs"
	}

	return "patches"
}

// GenerateNotes renders the release markdown for one planned release.
// assets may be nil when the release ships no artifacts.
func GenerateNotes(rel Release, categories []Category, repo string, assets *Assets) string {
	var sb strings.Builder

	version := rel.Tag
	date := time.Now().Format("2006-01-02")
	title := version
	if rel.Pkg.Name != "" {
		title = rel.Pkg.Name + " " + rel.Version
	}
	sb.WriteString(fmt.Sprintf("## %s — %s\n\n", title, date))

	// ── Locked mode: say which packages moved in this version ──────
	if len(rel.Changed) > 0 {
		sb.WriteString("**Packages:** " + strings.Join(rel.Changed, ", ") + "\n\n")
	}

	// ── Categorized entries with commit + PR links ─────────────────
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("### %s %s\n", cat.Emoji, cat.Name))
		for _, c := range cat.Commits {
			line := cleanTitle(c.Title)
			entry := "- " + line

			// Add PR link
			if c.PRNumber > 0 {
				entry += fmt.Sprintf(" ([#%d](https://github.com/%s/pull/%d))", c.PRNumber, repo, c.PRNumber)
			}

			// Add commit link — always include short SHA
			if c.SHA != "" {
				short := c.SHA
				if len(short) > 7 {
					short = short[:7]
				}
				entry += fmt.Sprintf(" — [`%s`](https://github.com/%s/commit/%s)", short, repo, c.SHA)
			}

			sb.WriteString(entry + "\n")
		}
		sb.WriteString("\n")
	}

	// ── Dependency-only releases have no commits of their own ──────
	if len(categories) == 0 {
		sb.WriteString("_Released for a " + rel.Reason + " — no source changes in this package._\n\n")
	}

	// ── PR summary table ───────────────────────────────────────────
	var prCommits []CommitInfo
	seen := map[int]bool{}
	for _, cat := range categories {
		for _, c := range cat.Commits {
			if c.PRNumber > 0 && !seen[c.PRNumber] {
				seen[c.PRNumber] = true
				prCommits = append(prCommits, c)
			}
		}
	}

	if len(prCommits) > 0 {
		sb.WriteString("---\n\n")
		sb.WriteString("### 📋 Pull Requests\n\n")
		sb.WriteString("| PR | Title | Author |\n")
		sb.WriteString("|----|-------|--------|\n")
		for _, c := range prCommits {
			author := ""
			if c.Author != "" {
				author = "@" + c.Author
			}
			sb.WriteString(fmt.Sprintf("| [#%d](https://github.com/%s/pull/%d) | %s | %s |\n",
				c.PRNumber, repo, c.PRNumber, cleanTitle(c.Title), author))
		}
		sb.WriteString("\n")
	}

	// ── Assets: container image + binary downloads ─────────────────
	if assets != nil && (assets.ImageRef != "" || len(assets.Files) > 0) {
		sb.WriteString("### 📦 Assets\n\n")

		if assets.ImageRef != "" {
			// ghcr.io/owner/repo[/pkg]:tag → package page lives at
			// github.com/owner/repo/pkgs/container/<name>
			name := assets.ImageRef
			if i := strings.LastIndex(name, "/"); i >= 0 {
				name = name[i+1:]
			}
			if i := strings.Index(name, ":"); i >= 0 {
				name = name[:i]
			}
			sb.WriteString(fmt.Sprintf("- 🐳 Container image: [`%s`](https://github.com/%s/pkgs/container/%s) — `docker pull %s`\n",
				assets.ImageRef, repo, name, assets.ImageRef))
		}

		for _, f := range assets.Files {
			sb.WriteString(fmt.Sprintf("- [%s](https://github.com/%s/releases/download/%s/%s)\n",
				f.Name, repo, version, f.Name))
		}
		sb.WriteString("\n")
	}

	// ── Full changelog link ────────────────────────────────────────
	if prevTag := rel.PrevTag; prevTag != "" && prevTag != "v0.0.0" && prevTag != "0.0.0" {
		sb.WriteString(fmt.Sprintf("**Full changelog:** [`%s...%s`](https://github.com/%s/compare/%s...%s)\n\n",
			prevTag, version, repo, prevTag, version))
	}

	return sb.String()
}

// UpdateChangelog prepends the new release notes to CHANGELOG.md.
func UpdateChangelog(dir, notes string) error {
	path := filepath.Join(dir, "CHANGELOG.md")

	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	}

	insertMarker := "<!-- releases are appended here automatically by deploya release -->"
	var newContent string

	if strings.Contains(existing, insertMarker) {
		newContent = strings.Replace(existing, insertMarker,
			insertMarker+"\n\n"+notes, 1)
	} else {
		lines := strings.SplitN(existing, "\n", 2)
		if len(lines) == 2 {
			newContent = lines[0] + "\n\n" + notes + "\n" + lines[1]
		} else {
			newContent = notes + "\n" + existing
		}
	}

	return os.WriteFile(path, []byte(newContent), 0644)
}

// cleanTitle strips conventional commit prefix from title for display.
func cleanTitle(title string) string {
	prefixes := []string{
		"feat!:", "fix!:", "chore!:",
		"feat:", "fix:", "chore:", "docs:", "doc:", "perf:", "refactor:",
		"test:", "ci:", "build:", "style:",
		"feat(", "fix(", "chore(", "docs(", "perf(", "refactor(", "test(", "ci(",
	}
	lower := strings.ToLower(title)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			idx := strings.Index(title, ":")
			if idx >= 0 && idx+2 < len(title) {
				cleaned := strings.TrimSpace(title[idx+1:])
				if len(cleaned) > 0 {
					return strings.ToUpper(cleaned[:1]) + cleaned[1:]
				}
			}
		}
	}
	if len(title) > 0 {
		return strings.ToUpper(title[:1]) + title[1:]
	}
	return title
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}
