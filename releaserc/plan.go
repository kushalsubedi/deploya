package releaserc

import (
	"fmt"
	"sort"
	"strings"
)

// Release is one planned release: a unit, the version it moves to, and the
// commits that justify it.
type Release struct {
	Pkg     Package
	Paths   []string // git pathspec the commits were filtered by
	PrevTag string
	Tag     string // git tag, e.g. "api/v1.4.0"
	Version string // bare version, e.g. "v1.4.0" — safe for filenames and image tags
	Bump    BumpType
	Reason  string // why this unit is being released
	Commits []CommitInfo
	Changed []string // locked mode: package names with commits in this release
}

// Plan is the full set of releases a `deploya release` invocation would cut.
type Plan struct {
	Mode     string
	Releases []Release
	Skipped  []Skip
}

// Skip records a unit that was considered but has nothing to release.
type Skip struct {
	Name   string
	Reason string
}

// BuildPlan works out what should be released. only and group narrow the
// selection; both empty means every configured unit.
//
// Independent mode yields one release per changed package plus any package
// that depends on a changed one. Locked mode yields a single release covering
// every selected package path.
func BuildPlan(dir string, cfg Config, only []string, group string) (Plan, error) {
	units, err := selectUnits(cfg, only, group)
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{Mode: cfg.Mode}
	if cfg.Locked() {
		rel, skip, err := planLocked(dir, cfg, units)
		if err != nil {
			return Plan{}, err
		}
		if skip != nil {
			plan.Skipped = append(plan.Skipped, *skip)
		} else {
			plan.Releases = append(plan.Releases, *rel)
		}
		return plan, nil
	}

	changed := map[string]Release{}
	for _, u := range units {
		rel, skip, err := planUnit(dir, cfg, u, []string{u.Path}, "commits")
		if err != nil {
			return Plan{}, err
		}
		if skip != nil {
			plan.Skipped = append(plan.Skipped, *skip)
			continue
		}
		changed[strings.ToLower(u.Name)] = *rel
	}

	// Dependency cascade: a package whose dependency is being released gets a
	// patch bump so the two never drift apart. Repeated to a fixpoint so the
	// bump propagates transitively (A → B → C).
	if err := cascade(dir, cfg, changed); err != nil {
		return Plan{}, err
	}

	// Emit in configured order so output and tag creation are deterministic.
	for _, u := range cfg.Units() {
		if rel, ok := changed[strings.ToLower(u.Name)]; ok {
			plan.Releases = append(plan.Releases, rel)
		}
	}
	// Drop skips that the cascade later promoted into real releases.
	plan.Skipped = filterSkips(plan.Skipped, changed)
	return plan, nil
}

// planUnit resolves the previous tag, reads the commits under the unit's paths
// and works out the next version. Returns a Skip when there is nothing to do.
func planUnit(dir string, cfg Config, u Package, paths []string, reason string) (*Release, *Skip, error) {
	prevTag, err := LatestTagMatching(dir, tagGlob(u.TagPrefix))
	if err != nil {
		return nil, nil, fmt.Errorf("could not read git tags for %s: %w", u.Label(), err)
	}

	commits, err := GitLog(dir, prevTag, paths...)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read git log for %s: %w", u.Label(), err)
	}
	commits = Releasable(commits)
	if len(commits) == 0 {
		return nil, &Skip{Name: u.Label(), Reason: "no commits since " + displayTag(prevTag)}, nil
	}

	rel, err := versionFor(u, prevTag, DetermineBump(commits))
	if err != nil {
		return nil, nil, err
	}
	rel.Paths = paths
	rel.Reason = reason
	rel.Commits = commits
	return rel, nil, nil
}

func planLocked(dir string, cfg Config, units []Package) (*Release, *Skip, error) {
	root := Package{
		Path:           ".",
		TagPrefix:      cfg.TagPrefix,
		CurrentVersion: cfg.CurrentVersion,
		Archive:        cfg.Archive,
		Registry:       cfg.Registry,
	}

	var paths []string
	for _, u := range units {
		paths = append(paths, u.Path)
	}
	if !cfg.IsMonorepo() {
		paths = []string{"."}
	}

	rel, skip, err := planUnit(dir, cfg, root, paths, "locked release")
	if err != nil || skip != nil {
		return nil, skip, err
	}

	// Record which packages actually moved, for the release notes.
	if cfg.IsMonorepo() {
		for _, u := range units {
			touched, err := GitLog(dir, rel.PrevTag, u.Path)
			if err != nil {
				return nil, nil, err
			}
			if len(Releasable(touched)) > 0 {
				rel.Changed = append(rel.Changed, u.Name)
			}
		}
	}
	return rel, nil, nil
}

// cascade adds a patch release for every package that depends on one already
// being released. Runs to a fixpoint; the unit count bounds the iterations.
func cascade(dir string, cfg Config, changed map[string]Release) error {
	units := cfg.Units()
	for i := 0; i <= len(units); i++ {
		added := false
		for _, u := range units {
			key := strings.ToLower(u.Name)
			if _, ok := changed[key]; ok {
				continue
			}
			trigger := ""
			for _, dep := range u.DependsOn {
				if _, ok := changed[strings.ToLower(dep)]; ok {
					trigger = dep
					break
				}
			}
			if trigger == "" {
				continue
			}
			prevTag, err := LatestTagMatching(dir, tagGlob(u.TagPrefix))
			if err != nil {
				return err
			}
			rel, err := versionFor(u, prevTag, BumpPatch)
			if err != nil {
				return err
			}
			rel.Paths = []string{u.Path}
			rel.Reason = "dependency bump (" + trigger + ")"
			changed[key] = *rel
			added = true
		}
		if !added {
			return nil
		}
	}
	return nil
}

// versionFor resolves the unit's current version and applies the bump. The
// latest reachable tag wins over current_version, which is only a fallback for
// the first release.
func versionFor(u Package, prevTag string, bump BumpType) (*Release, error) {
	current, err := ParseVersion(TrimTagPrefix(u.CurrentVersion, u.TagPrefix))
	if err != nil {
		return nil, fmt.Errorf("invalid current_version for %s: %w", u.Label(), err)
	}
	if prevTag != "" {
		if tagVer, tagErr := ParseVersion(TrimTagPrefix(prevTag, u.TagPrefix)); tagErr == nil {
			current = tagVer
		}
	}
	if current.Prefix == "" {
		current.Prefix = "v"
	}

	next := current.Bump(bump)
	return &Release{
		Pkg:     u,
		PrevTag: prevTag,
		Tag:     u.TagPrefix + TrimTagPrefix(next.String(), u.TagPrefix),
		Version: next.String(),
		Bump:    bump,
	}, nil
}

// selectUnits narrows the configured units by --only and --group.
func selectUnits(cfg Config, only []string, group string) ([]Package, error) {
	names := append([]string{}, only...)
	if group != "" {
		members, ok := cfg.Groups[group]
		if !ok {
			return nil, fmt.Errorf("unknown group %q — configured groups: %s", group, strings.Join(groupNames(cfg), ", "))
		}
		names = append(names, members...)
	}
	if len(names) == 0 {
		return cfg.Units(), nil
	}
	if !cfg.IsMonorepo() {
		return nil, fmt.Errorf("--only/--group need a packages list in .releaserc")
	}

	seen := map[string]bool{}
	var units []Package
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[strings.ToLower(n)] {
			continue
		}
		u, ok := cfg.Unit(n)
		if !ok {
			return nil, fmt.Errorf("unknown package %q", n)
		}
		seen[strings.ToLower(n)] = true
		units = append(units, u)
	}
	return units, nil
}

// Releasable drops deploya's own release commits so a second run of
// `deploya release` with no new work is a no-op.
func Releasable(commits []CommitInfo) []CommitInfo {
	var out []CommitInfo
	for _, c := range commits {
		if isSkipCommit(c.Title) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// TrimTagPrefix strips the configured tag prefix and a plain leading "v" so
// the prefix can be re-applied exactly once.
func TrimTagPrefix(v, tagPrefix string) string {
	if tagPrefix != "" {
		v = strings.TrimPrefix(v, tagPrefix)
	}
	return strings.TrimPrefix(v, "v")
}

// tagGlob turns a tag prefix into a git tag match pattern. An empty prefix
// matches everything, which is the pre-monorepo behaviour.
func tagGlob(prefix string) string {
	if prefix == "" {
		return ""
	}
	return prefix + "*"
}

func displayTag(tag string) string {
	if tag == "" {
		return "the start of history"
	}
	return tag
}

func filterSkips(skips []Skip, changed map[string]Release) []Skip {
	var out []Skip
	for _, s := range skips {
		if _, ok := changed[strings.ToLower(s.Name)]; ok {
			continue
		}
		out = append(out, s)
	}
	return out
}

func groupNames(cfg Config) []string {
	var names []string
	for n := range cfg.Groups {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return []string{"(none)"}
	}
	return names
}
