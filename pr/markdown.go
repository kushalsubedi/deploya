package pr

import (
	"fmt"
	"strings"
)

// GenerateMarkdown creates a structured, aesthetic Pull Request description
// from the unmerged commits and diff statistics without requiring an external AI service.
func GenerateMarkdown(head, base, repo string, commits []Commit, diffStat string) PRContent {
	title := generateTitle(head, commits)
	body := generateBody(head, base, repo, commits, diffStat)
	return PRContent{
		Title: title,
		Body:  body,
	}
}

func generateTitle(head string, commits []Commit) string {
	if len(commits) == 1 {
		return commits[0].Title
	}

	// Determine dominant commit type
	typeCounts := make(map[string]int)
	for _, c := range commits {
		typeCounts[c.Type]++
	}

	primaryType := "feat"
	if typeCounts["fix"] > typeCounts["feat"] {
		primaryType = "fix"
	} else if typeCounts["refactor"] > typeCounts["feat"] {
		primaryType = "refactor"
	} else if typeCounts["docs"] > typeCounts["feat"] && len(typeCounts) == 1 {
		primaryType = "docs"
	} else if typeCounts["chore"] > typeCounts["feat"] && len(typeCounts) == 1 {
		primaryType = "chore"
	}

	// Clean branch name for title
	cleanBranch := head
	for _, prefix := range []string{"feature/", "feat/", "fix/", "bugfix/", "hotfix/", "chore/", "refactor/"} {
		if strings.HasPrefix(cleanBranch, prefix) {
			cleanBranch = strings.TrimPrefix(cleanBranch, prefix)
			break
		}
	}
	cleanBranch = strings.ReplaceAll(cleanBranch, "-", " ")
	cleanBranch = strings.ReplaceAll(cleanBranch, "_", " ")
	cleanBranch = strings.TrimSpace(cleanBranch)

	if cleanBranch != "" && cleanBranch != head {
		return fmt.Sprintf("%s: %s", primaryType, cleanBranch)
	}

	// If branch name isn't descriptive, summarize from first commit or commit count
	if len(commits) > 0 {
		return commits[0].Title
	}

	return fmt.Sprintf("merge %s into %s", head, head)
}

func generateBody(head, base, repo string, commits []Commit, diffStat string) string {
	var sb strings.Builder

	sb.WriteString("## 🎯 Overview\n")
	sb.WriteString(fmt.Sprintf("This pull request merges **%d unmerged commit(s)** from `%s` into `%s`.\n\n", len(commits), head, base))

	// Categorize commits
	categories := []struct {
		Title string
		Emoji string
		Types []string
	}{
		{"Features", "✨", []string{"feat", "feature"}},
		{"Bug Fixes", "🐛", []string{"fix", "bugfix", "bug"}},
		{"Refactoring & Performance", "⚡", []string{"refactor", "perf", "improvement"}},
		{"Documentation", "📝", []string{"docs", "doc"}},
		{"Tests", "🧪", []string{"test", "tests"}},
		{"Maintenance & CI", "🔧", []string{"chore", "ci", "build", "patch"}},
	}

	categorized := make(map[string][]Commit)
	for _, c := range commits {
		matched := false
		for _, cat := range categories {
			for _, t := range cat.Types {
				if strings.EqualFold(c.Type, t) {
					categorized[cat.Title] = append(categorized[cat.Title], c)
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			categorized["Other Changes"] = append(categorized["Other Changes"], c)
		}
	}

	hasKeyChanges := false
	for _, cat := range categories {
		list, ok := categorized[cat.Title]
		if ok && len(list) > 0 {
			if !hasKeyChanges {
				sb.WriteString("## 🛠️ Key Changes\n\n")
				hasKeyChanges = true
			}
			sb.WriteString(fmt.Sprintf("### %s %s\n", cat.Emoji, cat.Title))
			for _, c := range list {
				scope := ""
				if c.Scope != "" {
					scope = fmt.Sprintf("**(%s)** ", c.Scope)
				}
				sb.WriteString(fmt.Sprintf("- %s%s ([`%s`](https://github.com/%s/commit/%s))\n",
					scope, c.Title, c.Short, repo, c.SHA))
			}
			sb.WriteString("\n")
		}
	}

	if list, ok := categorized["Other Changes"]; ok && len(list) > 0 {
		if !hasKeyChanges {
			sb.WriteString("## 🛠️ Key Changes\n\n")
		}
		sb.WriteString("### 📦 Other Commits\n")
		for _, c := range list {
			sb.WriteString(fmt.Sprintf("- %s ([`%s`](https://github.com/%s/commit/%s))\n",
				c.Title, c.Short, repo, c.SHA))
		}
		sb.WriteString("\n")
	}

	// Diff stat summary if available
	if diffStat != "" {
		sb.WriteString("## 📊 Diff Summary\n\n")
		sb.WriteString("```\n")
		sb.WriteString(diffStat)
		sb.WriteString("\n```\n\n")
	}

	// Verification checklist
	sb.WriteString("## 🧪 Verification & Checklist\n\n")
	sb.WriteString("- [x] Commits reviewed and verified\n")
	sb.WriteString("- [ ] Automated CI tests pass\n")
	sb.WriteString("- [ ] Code changes follow repository standards\n\n")

	sb.WriteString("---\n")
	sb.WriteString("<sub>Generated automatically by [Deploya](https://github.com/kushalsubedi/deploya)</sub>\n")

	return sb.String()
}
