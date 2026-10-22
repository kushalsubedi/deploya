package detector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkspacePkg is one package discovered in a monorepo workspace.
type WorkspacePkg struct {
	Name      string   // short, tag-friendly name (scope stripped)
	Path      string   // slash-separated path relative to the repo root
	Lang      string   // node, go, rust
	DependsOn []string // short names of sibling packages it depends on
}

// DetectWorkspace finds the packages of a monorepo by reading whichever
// workspace manifest the repo uses: go.work, pnpm-workspace.yaml, the
// package.json "workspaces" field, lerna.json or a Cargo workspace.
// Returns nil when the repo is a single project.
func DetectWorkspace(dir string) []WorkspacePkg {
	var pkgs []WorkspacePkg
	for _, p := range goWorkspace(dir) {
		pkgs = append(pkgs, p)
	}
	for _, p := range nodeWorkspace(dir) {
		pkgs = append(pkgs, p)
	}
	for _, p := range cargoWorkspace(dir) {
		pkgs = append(pkgs, p)
	}

	pkgs = dedupeByPath(pkgs)
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })
	if len(pkgs) < 2 {
		// One package is not a monorepo — treat it as a plain project.
		return nil
	}
	return pkgs
}

// ── Go ──────────────────────────────────────────────────────────────────────

func goWorkspace(dir string) []WorkspacePkg {
	b, err := os.ReadFile(filepath.Join(dir, "go.work"))
	if err != nil {
		return nil
	}

	var paths []string
	inBlock := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(stripComment(line))
		switch {
		case line == "use (":
			inBlock = true
		case inBlock && line == ")":
			inBlock = false
		case inBlock && line != "":
			paths = append(paths, unquote(line))
		case strings.HasPrefix(line, "use "):
			paths = append(paths, unquote(strings.TrimSpace(strings.TrimPrefix(line, "use "))))
		}
	}

	// First pass: module path per package, so requires can be resolved to
	// sibling packages rather than external dependencies.
	modules := map[string]string{} // module path → short name
	var pkgs []WorkspacePkg
	for _, p := range paths {
		rel := cleanRel(p)
		mod := goModulePath(filepath.Join(dir, rel))
		if mod == "" {
			continue
		}
		name := shortName(mod, rel)
		modules[mod] = name
		pkgs = append(pkgs, WorkspacePkg{Name: name, Path: rel, Lang: "go"})
	}

	for i, p := range pkgs {
		pkgs[i].DependsOn = goDeps(filepath.Join(dir, p.Path), modules, p.Name)
	}
	return pkgs
}

func goModulePath(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(stripComment(line))
		if strings.HasPrefix(line, "module ") {
			return unquote(strings.TrimSpace(strings.TrimPrefix(line, "module ")))
		}
	}
	return ""
}

func goDeps(dir string, modules map[string]string, self string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil
	}
	var deps []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(stripComment(line))
		line = strings.TrimPrefix(line, "require ")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name, ok := modules[unquote(fields[0])]
		if !ok || name == self || seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, name)
	}
	return deps
}

// ── Node (npm / yarn / pnpm / lerna) ────────────────────────────────────────

func nodeWorkspace(dir string) []WorkspacePkg {
	globs := nodeGlobs(dir)
	if len(globs) == 0 {
		return nil
	}

	names := map[string]string{} // package.json name → short name
	var pkgs []WorkspacePkg
	for _, rel := range expandGlobs(dir, globs) {
		name, _ := nodePackageName(filepath.Join(dir, rel))
		if name == "" {
			continue
		}
		short := shortName(name, rel)
		names[name] = short
		pkgs = append(pkgs, WorkspacePkg{Name: short, Path: rel, Lang: "node"})
	}

	for i, p := range pkgs {
		pkgs[i].DependsOn = nodeDeps(filepath.Join(dir, p.Path), names, p.Name)
	}
	return pkgs
}

// nodeGlobs collects workspace globs from whichever node tool declares them.
func nodeGlobs(dir string) []string {
	var globs []string

	if b, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil {
		var ws struct {
			Packages []string `yaml:"packages"`
		}
		if yaml.Unmarshal(b, &ws) == nil {
			globs = append(globs, ws.Packages...)
		}
	}

	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Workspaces json.RawMessage `json:"workspaces"`
		}
		if json.Unmarshal(b, &pkg) == nil && len(pkg.Workspaces) > 0 {
			// "workspaces" is either a list or {"packages": [...]}
			var list []string
			if json.Unmarshal(pkg.Workspaces, &list) == nil {
				globs = append(globs, list...)
			} else {
				var obj struct {
					Packages []string `json:"packages"`
				}
				if json.Unmarshal(pkg.Workspaces, &obj) == nil {
					globs = append(globs, obj.Packages...)
				}
			}
		}
	}

	if b, err := os.ReadFile(filepath.Join(dir, "lerna.json")); err == nil {
		var lerna struct {
			Packages []string `json:"packages"`
		}
		if json.Unmarshal(b, &lerna) == nil {
			if len(lerna.Packages) == 0 {
				lerna.Packages = []string{"packages/*"}
			}
			globs = append(globs, lerna.Packages...)
		}
	}

	return globs
}

func nodePackageName(dir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", false
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return "", false
	}
	return pkg.Name, pkg.Name != ""
}

func nodeDeps(dir string, names map[string]string, self string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return nil
	}

	var deps []string
	seen := map[string]bool{}
	for _, set := range []map[string]string{pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies} {
		for dep := range set {
			short, ok := names[dep]
			if !ok || short == self || seen[short] {
				continue
			}
			seen[short] = true
			deps = append(deps, short)
		}
	}
	sort.Strings(deps)
	return deps
}

// ── Rust ────────────────────────────────────────────────────────────────────

func cargoWorkspace(dir string) []WorkspacePkg {
	b, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if err != nil || !strings.Contains(string(b), "[workspace]") {
		return nil
	}

	// members = [...] may span several lines; collect until the closing
	// bracket rather than pulling in a TOML parser for one field.
	var globs []string
	collecting := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(stripHashComment(line))
		if !collecting && strings.HasPrefix(line, "members") {
			if i := strings.Index(line, "["); i >= 0 {
				collecting = true
				line = line[i+1:]
			}
		} else if !collecting {
			continue
		}
		if i := strings.Index(line, "]"); i >= 0 {
			line = line[:i]
			collecting = false
		}
		for _, part := range strings.Split(line, ",") {
			if v := unquote(strings.TrimSpace(part)); v != "" {
				globs = append(globs, v)
			}
		}
		if !collecting {
			break
		}
	}

	var pkgs []WorkspacePkg
	for _, rel := range expandGlobs(dir, globs) {
		if !fileExists(filepath.Join(dir, rel, "Cargo.toml")) {
			continue
		}
		pkgs = append(pkgs, WorkspacePkg{Name: filepath.Base(rel), Path: rel, Lang: "rust"})
	}
	return pkgs
}

// ── shared helpers ──────────────────────────────────────────────────────────

// expandGlobs resolves workspace globs to existing directories, relative to
// the repo root. "**" is treated as a single level — deeper nesting is rare
// enough that the user can list those paths explicitly.
func expandGlobs(dir string, globs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "" || strings.HasPrefix(g, "!") {
			continue
		}
		g = strings.ReplaceAll(g, "**", "*")
		matches, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(g)))
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || !info.IsDir() {
				continue
			}
			rel, err := filepath.Rel(dir, m)
			if err != nil || seen[rel] {
				continue
			}
			seen[rel] = true
			out = append(out, filepath.ToSlash(rel))
		}
	}
	return out
}

// shortName turns a module path or scoped package name into a tag-friendly
// name, falling back to the directory name.
func shortName(full, relPath string) string {
	name := full
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimPrefix(name, "@")
	if name == "" {
		name = filepath.Base(relPath)
	}
	return name
}

func dedupeByPath(pkgs []WorkspacePkg) []WorkspacePkg {
	seen := map[string]bool{}
	var out []WorkspacePkg
	for _, p := range pkgs {
		if p.Path == "" || p.Path == "." || seen[p.Path] {
			continue
		}
		seen[p.Path] = true
		out = append(out, p)
	}
	return out
}

func cleanRel(p string) string {
	p = strings.TrimPrefix(filepath.ToSlash(p), "./")
	return strings.TrimSuffix(p, "/")
}

func stripComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

func stripHashComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"'`)
}
