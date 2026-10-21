// Derived from anyflip-downloader (github.com/Lofter1/anyflip-downloader),
// Copyright (C) Lofter1 and contributors, licensed under the GNU General
// Public License v3.0. Modified 2026: writes to a temp file and renames
// on success instead of silently succeeding when the target exists or
// swallowing per-chunk errors. See LICENSE-anyflip-downloader for the
// original license text.

package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

// buildPDF merges every image in imageDir into a single PDF at
// outputPath, converting chunkSize images at a time. Progress reports
// the running page count.
func buildPDF(ctx context.Context, outputPath, imageDir string, chunkSize int, progress ProgressFunc) (string, error) {
	if chunkSize < 1 {
		chunkSize = 10
	}

	files, err := os.ReadDir(imageDir)
	if err != nil {
		return "", err
	}

	var imagePaths []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if supportedImageExt(filepath.Ext(file.Name())) {
			imagePaths = append(imagePaths, filepath.Join(imageDir, file.Name()))
		}
	}
	if len(imagePaths) == 0 {
		return "", fmt.Errorf("no downloaded images found in %s", imageDir)
	}

	outputPath = uniquePath(outputPath)
	tmpPath := outputPath + ".tmp"
	defer os.Remove(tmpPath)

	impConf := pdfcpu.DefaultImportConfig()
	for i := 0; i < len(imagePaths); i += chunkSize {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		end := min(i+chunkSize, len(imagePaths))
		if err := api.ImportImagesFile(imagePaths[i:end], tmpPath, impConf, nil); err != nil {
			return "", fmt.Errorf("converting pages %d-%d: %w", i+1, end, err)
		}
		if progress != nil {
			progress(Progress{Phase: PhaseConverting, Done: end, Total: len(imagePaths)})
		}
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}

// supportedImageExt reports whether the PDF builder can import ext
// (case-insensitive, with leading dot).
func supportedImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}

// uniquePath appends " (2)", " (3)", ... before the extension if path
// already exists, so a finished download never clobbers a prior one.
func uniquePath(path string) string {
	if _, err := os.Stat(path); err != nil {
		return path
	}
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
}
