package pr

// Commit represents a git commit with its hash, message, author, and category.
type Commit struct {
	SHA     string // Full commit SHA
	Short   string // 7-char short SHA
	Message string // Full commit message
	Title   string // First line / subject of commit
	Author  string // Commit author name
	Type    string // Conventional commit type (feat, fix, docs, chore, etc.)
	Scope   string // Commit scope if present (e.g. "ci", "ui")
}

// PRContent holds the generated title and markdown body for a Pull Request.
type PRContent struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// PullRequestResult represents the created or existing GitHub Pull Request.
type PullRequestResult struct {
	ID        int64  `json:"id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	Draft     bool   `json:"draft"`
	Head      string `json:"head"`
	Base      string `json:"base"`
	CreatedAt string `json:"created_at"`
}
