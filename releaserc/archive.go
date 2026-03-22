package releaserc

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Target defines a build target platform.
type Target struct {
	OS   string
	Arch string
}

var defaultTargets = []Target{
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"},
}

// BuildArchives cross-compiles the Go binary for all platforms
// and returns a list of archive file paths.
func BuildArchives(dir, repo, version string) ([]string, error) {
	// Get binary name from repo (owner/repo → repo)
	parts := strings.SplitN(repo, "/", 2)
	binaryName := repo
	if len(parts) == 2 {
		binaryName = parts[1]
	}

	// Create temp output dir
	outDir := filepath.Join(dir, ".deploya-dist")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("could not create dist dir: %w", err)
	}
	defer os.RemoveAll(outDir) // clean up after upload

	var archives []string

	for _, t := range defaultTargets {
		archivePath, err := buildOne(dir, outDir, binaryName, version, t)
		if err != nil {
			fmt.Printf("   ⚠️  Skipping %s/%s: %v\n", t.OS, t.Arch, err)
			continue
		}
		archives = append(archives, archivePath)
		fmt.Printf("   ✅ Built: %s\n", filepath.Base(archivePath))
	}

	return archives, nil
}

func buildOne(dir, outDir, binaryName, version string, t Target) (string, error) {
	// Binary file name
	binName := binaryName
	if t.OS == "windows" {
		binName += ".exe"
	}

	binPath := filepath.Join(outDir, fmt.Sprintf("%s-%s-%s-%s", binaryName, version, t.OS, t.Arch))
	if t.OS == "windows" {
		binPath += ".exe"
	}

	// Run go build with cross-compilation env
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOOS="+t.OS,
		"GOARCH="+t.Arch,
		"CGO_ENABLED=0",
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build failed: %s", string(out))
	}

	// Package into archive
	archiveName := fmt.Sprintf("%s-%s-%s-%s", binaryName, version, t.OS, t.Arch)
	var archivePath string
	var err error

	if t.OS == "windows" {
		archivePath = filepath.Join(outDir, archiveName+".zip")
		err = createZip(archivePath, binPath, binName)
	} else {
		archivePath = filepath.Join(outDir, archiveName+".tar.gz")
		err = createTarGz(archivePath, binPath, binName)
	}

	if err != nil {
		return "", fmt.Errorf("archive failed: %w", err)
	}

	// Remove raw binary — only keep archive
	os.Remove(binPath)

	return archivePath, nil
}

func createTarGz(archivePath, filePath, fileName string) error {
	out, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:    fileName,
		Size:    info.Size(),
		Mode:    0755,
		ModTime: info.ModTime(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, f)
	return err
}

func createZip(archivePath, filePath, fileName string) error {
	out, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := zw.Create(fileName)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, f)
	return err
}
