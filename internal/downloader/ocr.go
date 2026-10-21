package downloader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
)

// ErrOCRUnavailable is returned when ExtractText is set but no working
// Tesseract install was configured (see Options.TesseractPath).
var ErrOCRUnavailable = errors.New("OCR is not available: the bundled Tesseract folder is missing")

// extractText runs Tesseract over every downloaded page image and
// concatenates the results, in page order, into a single text file.
func extractText(ctx context.Context, imagesDir, outDir, title string, opt Options, progress ProgressFunc) (string, error) {
	if opt.TesseractPath == "" {
		return "", ErrOCRUnavailable
	}
	if _, err := os.Stat(opt.TesseractPath); err != nil {
		return "", ErrOCRUnavailable
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return "", err
	}
	var pages []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch filepath.Ext(e.Name()) {
		case ".jpg", ".jpeg", ".png", ".webp":
			pages = append(pages, filepath.Join(imagesDir, e.Name()))
		}
	}
	sort.Strings(pages) // filenames are zero-padded page numbers, e.g. 0001.jpg

	threads := opt.Threads
	if threads < 1 {
		threads = 1
	}

	texts := make([]string, len(pages))
	errs := make([]error, len(pages))

	indices := make(chan int)
	go func() {
		defer close(indices)
		for i := range pages {
			select {
			case indices <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	var done int
	var mu sync.Mutex
	wg.Add(threads)
	for w := 0; w < threads; w++ {
		go func() {
			defer wg.Done()
			for i := range indices {
				texts[i], errs[i] = runTesseract(ctx, opt.TesseractPath, opt.TessdataDir, pages[i])
				mu.Lock()
				done++
				d := done
				mu.Unlock()
				if progress != nil {
					progress(Progress{Phase: PhaseOCR, Done: d, Total: len(pages)})
				}
			}
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return "", err
	}
	for i, err := range errs {
		if err != nil {
			return "", fmt.Errorf("OCR failed on page %d: %w", i+1, err)
		}
	}

	var buf bytes.Buffer
	for i, t := range texts {
		if i > 0 {
			buf.WriteString("\n\n----- page break -----\n\n")
		}
		buf.WriteString(t)
	}

	textPath := uniquePath(filepath.Join(outDir, title+".txt"))
	return textPath, os.WriteFile(textPath, buf.Bytes(), 0o644)
}

// runTesseract OCRs a single image, using tesseract's "stdout" output
// base so no extra temp file is needed.
func runTesseract(ctx context.Context, tesseractPath, tessdataDir, imagePath string) (string, error) {
	args := []string{imagePath, "stdout", "-l", "eng"}
	if tessdataDir != "" {
		args = append(args, "--tessdata-dir", tessdataDir)
	}

	cmd := exec.CommandContext(ctx, tesseractPath, args...)
	// Bundled Linux/macOS tesseract ships its shared-library deps next to
	// the binary (see scripts/package-linux-release.sh and
	// package-macos-release.sh); point each platform's loader at them.
	// Harmless on Windows, where DLLs are found next to the exe anyway.
	libDir := filepath.Dir(tesseractPath)
	cmd.Env = append(os.Environ(),
		"LD_LIBRARY_PATH="+prependPath(libDir, os.Getenv("LD_LIBRARY_PATH")),
		"DYLD_LIBRARY_PATH="+prependPath(libDir, os.Getenv("DYLD_LIBRARY_PATH")),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// prependPath puts dir before existing, adding the separator only when
// existing is non-empty (an empty entry means the current directory).
func prependPath(dir, existing string) string {
	if existing == "" {
		return dir
	}
	return dir + ":" + existing
}
