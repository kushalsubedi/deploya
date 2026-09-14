package releaserc

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Assets describes the artifacts produced for a release, used to render
// the Assets section of the release notes and to upload files afterwards.
type Assets struct {
	ImageRef string      // full image ref incl. tag, e.g. ghcr.io/owner/repo:v1.2.3 — empty if none
	Files    []AssetFile // binary archives + checksums
}

// AssetFile is a single downloadable release asset.
type AssetFile struct {
	Name string // file name as it appears on the release page
	Path string // local path to upload from — empty when only planned (dry run)
}

// archiveTarget is one GOOS/GOARCH combination to build for.
type archiveTarget struct {
	goos, goarch string
}

var archiveTargets = []archiveTarget{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
}

const checksumsFile = "checksums.txt"

// PlanArchives returns the asset file names a release would produce,
// without building anything. Used for dry runs and notes previews.
func PlanArchives(repo, tag string) []AssetFile {
	name := repoName(repo)
	var files []AssetFile
	for _, t := range archiveTargets {
		files = append(files, AssetFile{Name: archiveName(name, tag, t)})
	}
	files = append(files, AssetFile{Name: checksumsFile})
	return files
}

// BuildArchives cross-compiles the project for all targets and packages each
// binary into a tar.gz (zip on windows), plus a sha256 checksums file.
// Currently supports Go projects only. Archives are written to a temp dir;
// the returned paths stay valid for the rest of the process.
func BuildArchives(dir, repo, tag string) ([]AssetFile, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return nil, fmt.Errorf("archive builds currently support Go projects only (no go.mod in %s)", dir)
	}
	if _, err := exec.LookPath("go"); err != nil {
		return nil, fmt.Errorf("go is not installed or not in PATH")
	}

	name := repoName(repo)
	outDir, err := os.MkdirTemp("", "deploya-dist-*")
	if err != nil {
		return nil, fmt.Errorf("could not create dist dir: %w", err)
	}

	// -X flags for symbols a project doesn't define are silently ignored,
	// so stamping several common version vars is safe for any Go project.
	ldflags := fmt.Sprintf(
		"-s -w -X main.version=%[1]s -X main.Version=%[1]s -X github.com/kushalsubedi/deploya/cmd.Version=%[1]s",
		tag,
	)

	var files []AssetFile
	for _, t := range archiveTargets {
		binName := name
		if t.goos == "windows" {
			binName += ".exe"
		}
		binPath := filepath.Join(outDir, fmt.Sprintf("%s_%s_%s", binName, t.goos, t.goarch))

		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", binPath, ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"CGO_ENABLED=0",
			"GOOS="+t.goos,
			"GOARCH="+t.goarch,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("build failed for %s/%s: %w\n%s", t.goos, t.goarch, err, strings.TrimSpace(string(out)))
		}

		archivePath := filepath.Join(outDir, archiveName(name, tag, t))
		if t.goos == "windows" {
			err = zipFile(archivePath, binPath, binName)
		} else {
			err = tarGzFile(archivePath, binPath, binName)
		}
		if err != nil {
			return nil, fmt.Errorf("could not package %s/%s: %w", t.goos, t.goarch, err)
		}

		files = append(files, AssetFile{Name: filepath.Base(archivePath), Path: archivePath})
	}

	sumsPath, err := writeChecksums(outDir, files)
	if err != nil {
		return nil, err
	}
	files = append(files, AssetFile{Name: checksumsFile, Path: sumsPath})

	return files, nil
}

// PlanImageRef returns the image ref a release would push, or "" if the
// project has no Dockerfile or the registry is unsupported.
func PlanImageRef(dir string, cfg Config, tag string) string {
	if !hasDockerfile(dir) || cfg.Registry != "ghcr" {
		return ""
	}
	return ghcrRef(cfg.GithubRepo) + ":" + tag
}

// BuildAndPushImage builds the project's Dockerfile and pushes it to the
// configured registry tagged with the release tag and :latest.
// Only GHCR is supported for now — it works with the built-in GITHUB_TOKEN.
func BuildAndPushImage(dir string, cfg Config, tag, token string) (string, error) {
	if !hasDockerfile(dir) {
		return "", fmt.Errorf("no Dockerfile found in %s", dir)
	}
	if cfg.Registry != "ghcr" {
		return "", fmt.Errorf("registry %q is not supported for release pushes yet — only ghcr for now", cfg.Registry)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker is not installed or not in PATH")
	}

	owner := strings.ToLower(strings.SplitN(cfg.GithubRepo, "/", 2)[0])
	ref := ghcrRef(cfg.GithubRepo)
	tagged := ref + ":" + tag
	latest := ref + ":latest"

	// Login with the release token — works with the built-in GITHUB_TOKEN
	// as long as the workflow has `packages: write` permission.
	login := exec.Command("docker", "login", "ghcr.io", "-u", owner, "--password-stdin")
	login.Stdin = strings.NewReader(token)
	if out, err := login.CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker login to ghcr.io failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	build := exec.Command("docker", "build", "-t", tagged, "-t", latest, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker build failed: %w\n%s", err, tail(string(out), 2000))
	}

	for _, t := range []string{tagged, latest} {
		push := exec.Command("docker", "push", t)
		if out, err := push.CombinedOutput(); err != nil {
			return "", fmt.Errorf("docker push %s failed: %w\n%s", t, err, tail(string(out), 2000))
		}
	}

	return tagged, nil
}

// ── internal ────────────────────────────────────────────────────────────────

// repoName returns the repo part of "owner/repo".
func repoName(repo string) string {
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		return repo[i+1:]
	}
	return repo
}

// ghcrRef returns the untagged GHCR image ref for a repo, lowercased as
// required by the registry.
func ghcrRef(repo string) string {
	return "ghcr.io/" + strings.ToLower(repo)
}

func archiveName(name, tag string, t archiveTarget) string {
	ext := ".tar.gz"
	if t.goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s%s", name, tag, t.goos, t.goarch, ext)
}

func hasDockerfile(dir string) bool {
	for _, f := range []string{"Dockerfile", "dockerfile", "Dockerfile.prod", "Dockerfile.production"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

func tarGzFile(dst, src, nameInArchive string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name: nameInArchive,
		Mode: 0o755,
		Size: info.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

func zipFile(dst, src, nameInArchive string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	w, err := zw.Create(nameInArchive)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func writeChecksums(outDir string, files []AssetFile) (string, error) {
	var sb strings.Builder
	for _, af := range files {
		f, err := os.Open(af.Path)
		if err != nil {
			return "", err
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
		sb.WriteString(fmt.Sprintf("%x  %s\n", h.Sum(nil), af.Name))
	}
	path := filepath.Join(outDir, checksumsFile)
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// tail returns the last n bytes of s — keeps huge docker output readable.
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
