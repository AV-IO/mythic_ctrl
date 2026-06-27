package web

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxUploadBytes caps the size of an uploaded archive.
	maxUploadBytes = 1 << 30 // 1 GiB
	// maxUnzippedFileBytes caps a single extracted file, as a zip-bomb guard.
	maxUnzippedFileBytes = 1 << 30 // 1 GiB
)

// unzip extracts the zip at zipPath into destDir. It guards against path
// traversal ("zip slip") by ensuring every entry resolves inside destDir, and
// against zip bombs by capping each extracted file's size.
func unzip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	cleanDest := filepath.Clean(destDir)
	prefix := cleanDest + string(os.PathSeparator)

	for _, f := range zr.File {
		target := filepath.Clean(filepath.Join(destDir, f.Name))
		if target != cleanDest && !strings.HasPrefix(target, prefix) {
			return fmt.Errorf("unsafe path in archive: %q", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()|0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	// CopyN with limit+1 lets us detect (and reject) oversized entries.
	n, err := io.CopyN(out, rc, maxUnzippedFileBytes+1)
	if err != nil && err != io.EOF {
		return err
	}
	if n > maxUnzippedFileBytes {
		return fmt.Errorf("file too large in archive: %q", f.Name)
	}
	return nil
}

// installRoot picks the folder to hand to InstallFolder. A GitHub/GitLab zip
// wraps everything in a single top-level directory (e.g. "apollo-main/"), so if
// the extraction contains exactly one real directory we install that; otherwise
// we install the extraction root. Metadata entries (dotfiles, __MACOSX) are
// ignored when deciding.
func installRoot(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var real []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if name == "__MACOSX" || strings.HasPrefix(name, ".") {
			continue
		}
		real = append(real, e)
	}
	if len(real) == 1 && real[0].IsDir() {
		return filepath.Join(dir, real[0].Name()), nil
	}
	return dir, nil
}
