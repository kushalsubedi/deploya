package releaserc

// CommitInfo holds data about a single commit enriched with GitHub PR data.
type CommitInfo struct {
	SHA      string // git commit SHA
	Message  string // raw commit message
	Title    string // PR title if found, otherwise commit message
	PRNumber int    // GitHub PR number, 0 if direct commit
	Label    string // primary GitHub label on the PR (e.g. "bug", "enhancement")
	Author   string // commit author
}

// Category holds a group of commits under a release section.
type Category struct {
	Name    string       // e.g. "Features"
	Emoji   string       // e.g. "✨"
	Commits []CommitInfo
}