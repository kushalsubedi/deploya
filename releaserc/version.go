package releaserc

import (
	"fmt"
	"strconv"
	"strings"
)

// BumpType represents the type of version bump.
type BumpType string

const (
	BumpNone  BumpType = "none"
	BumpPatch BumpType = "patch"
	BumpMinor BumpType = "minor"
	BumpMajor BumpType = "major"
)

// Version represents a parsed semantic version.
type Version struct {
	Major  int
	Minor  int
	Patch  int
	Prefix string // e.g. "v"
}

// ParseVersion parses a version string like "v1.2.3" or "1.2.3".
func ParseVersion(s string) (Version, error) {
	prefix := ""
	if strings.HasPrefix(s, "v") {
		prefix = "v"
		s = strings.TrimPrefix(s, "v")
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version format %q — expected major.minor.patch", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version %q", parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version %q", parts[2])
	}

	return Version{
		Major:  major,
		Minor:  minor,
		Patch:  patch,
		Prefix: prefix,
	}, nil
}

// String returns the version as a string e.g. "v1.2.3".
func (v Version) String() string {
	return fmt.Sprintf("%s%d.%d.%d", v.Prefix, v.Major, v.Minor, v.Patch)
}

// Bump returns a new version bumped by the given type.
func (v Version) Bump(t BumpType) Version {
	next := Version{Prefix: v.Prefix, Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	switch t {
	case BumpMajor:
		next.Major++
		next.Minor = 0
		next.Patch = 0
	case BumpMinor:
		next.Minor++
		next.Patch = 0
	case BumpPatch:
		next.Patch++
	}
	return next
}

// DetermineBump looks at all commits and returns the highest bump type needed.
//
// Rules:
//
//	BREAKING CHANGE in title or label → major
//	feat / feature                    → minor
//	fix / bugfix / bug                → patch
//	chore / refactor / perf / docs    → patch
//	anything else                     → patch
func DetermineBump(commits []CommitInfo) BumpType {
	bump := BumpPatch // default to patch

	for _, c := range commits {
		b := bumpForCommit(c)
		bump = higherBump(bump, b)
		// major is the highest — no need to check further
		if bump == BumpMajor {
			return bump
		}
	}

	return bump
}

func bumpForCommit(c CommitInfo) BumpType {
	// Check for breaking change first — highest priority
	if isBreaking(c) {
		return BumpMajor
	}

	// Check label first (Option C — labels over prefix)
	if c.Label != "" {
		switch strings.ToLower(c.Label) {
		case "enhancement", "feature", "feat":
			return BumpMinor
		case "bug", "fix", "bugfix":
			return BumpPatch
		}
	}

	// Fall back to commit/PR title prefix
	title := strings.ToLower(c.Title)
	switch {
	case hasPrefix(title, "feat", "feature"):
		return BumpMinor
	case hasPrefix(title, "fix", "bugfix", "bug"):
		return BumpPatch
	default:
		return BumpPatch
	}
}

func isBreaking(c CommitInfo) bool {
	title := strings.ToUpper(c.Title)
	if strings.Contains(title, "BREAKING CHANGE") || strings.Contains(title, "BREAKING:") {
		return true
	}
	if strings.ToLower(c.Label) == "breaking" {
		return true
	}
	// Conventional commit breaking change marker: feat!: or fix!:
	if strings.Contains(c.Title, "!:") {
		return true
	}
	return false
}

func hasPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p+":") || strings.HasPrefix(s, p+"(") {
			return true
		}
	}
	return false
}

func higherBump(a, b BumpType) BumpType {
	order := map[BumpType]int{
		BumpNone:  0,
		BumpPatch: 1,
		BumpMinor: 2,
		BumpMajor: 3,
	}
	if order[b] > order[a] {
		return b
	}
	return a
}
