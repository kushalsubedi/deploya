package releaserc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CategorizeCommits groups commits into categories based on labels and prefixes.
func CategorizeCommits(commits []CommitInfo, cats Categories) []Category {
	features := Category{Name: "Features", Emoji: "✨"}
	fixes := Category{Name: "Bug Fixes", Emoji: "🐛"}
	patches := Category{Name: "Patches & Maintenance", Emoji: "🔧"}
	docs := Category{Name: "Docs", Emoji: "📝"}

	for _, c := range commits {
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

	// Fall back to title prefix
	title := strings.ToLower(c.Title)
	for _, p := range cats.Features {
		if strings.HasPrefix(title, p+":") || strings.HasPrefix(title, p+"(") {
			return "features"
		}
	}
	for _, p := range cats.Fixes {
		if strings.HasPrefix(title, p+":") || strings.HasPrefix(title, p+"(") {
			return "fixes"
		}
	}
	for _, p := range cats.Docs {
		if strings.HasPrefix(title, p+":") || strings.HasPrefix(title, p+"(") {
			return "docs"
		}
	}

	return "patches"
}

// GenerateNotes renders the release markdown for a given version.
func GenerateNotes(version string, categories []Category, prevVersion, repo string) string {
	var sb strings.Builder

	date := time.Now().Format("2006-01-02")
	sb.WriteString(fmt.Sprintf("## %s — %s\n\n", version, date))

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("### %s %s\n", cat.Emoji, cat.Name))
		for _, c := range cat.Commits {
			line := cleanTitle(c.Title)
			if c.PRNumber > 0 {
				sb.WriteString(fmt.Sprintf("- %s ([#%d](https://github.com/%s/pull/%d))\n",
					line, c.PRNumber, repo, c.PRNumber))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", line))
			}
		}
		sb.WriteString("\n")
	}

	if prevVersion != "" && prevVersion != "v0.0.0" {
		sb.WriteString(fmt.Sprintf("**Full changelog:** [`%s...%s`](https://github.com/%s/compare/%s...%s)\n",
			prevVersion, version, repo, prevVersion, version))
	}

	return sb.String()
}

// UpdateChangelog prepends the new release notes to CHANGELOG.md.
func UpdateChangelog(dir, notes string) error {
	path := filepath.Join(dir, "CHANGELOG.md")

	// Read existing content
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	}

	// Find insertion point — after the header comment
	insertMarker := "<!-- releases are appended here automatically by deploya release -->"
	var newContent string

	if strings.Contains(existing, insertMarker) {
		newContent = strings.Replace(existing, insertMarker,
			insertMarker+"\n\n"+notes, 1)
	} else {
		// No marker — just prepend after first line
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
// "feat: add discord" → "Add discord"
func cleanTitle(title string) string {
	prefixes := []string{
		"feat!:", "fix!:", "chore!:",
		"feat:", "fix:", "chore:", "docs:", "perf:", "refactor:",
		"feat(", "fix(", "chore(", "docs(", "perf(",
	}
	lower := strings.ToLower(title)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			// Find the colon and trim up to it
			idx := strings.Index(title, ":")
			if idx >= 0 && idx+2 < len(title) {
				cleaned := strings.TrimSpace(title[idx+1:])
				// Capitalize first letter
				if len(cleaned) > 0 {
					return strings.ToUpper(cleaned[:1]) + cleaned[1:]
				}
			}
		}
	}
	// Capitalize first letter of original
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
