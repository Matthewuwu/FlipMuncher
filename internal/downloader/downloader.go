// Package downloader adapts the download logic of anyflip-downloader
// (github.com/Lofter1/anyflip-downloader by Lofter1, GPLv3) into an
// importable, stateless package: no package-level globals, context
// cancellation throughout, and safe for concurrent use across different
// books. See LICENSE-anyflip-downloader for the original project's
// license text; this package is itself licensed GPL-3.0 as a combined
// work (see LICENSE).
package downloader

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	PhaseDownloading = "downloading"
	PhaseConverting  = "converting"
	PhaseOCR         = "ocr"
)

// Progress reports how far a Download call has gotten.
type Progress struct {
	Phase string
	Done  int
	Total int
}

// ProgressFunc is called from the downloading goroutines; implementations
// must be safe to call concurrently and should return quickly.
type ProgressFunc func(Progress)

// Options controls a single Download call. Zero values fall back to
// sensible defaults.
type Options struct {
	Title      string        // overrides the title from the site
	Threads    int           // parallel page downloads, default 1
	Retries    int           // per-page retry attempts, default 1
	RetryDelay time.Duration // wait between retries, default 1s
	ChunkSize  int           // images per PDF-import batch, default 10
	KeepImages bool          // keep the downloaded page images after building the PDF

	// CompressImages re-encodes every page as JPEG at JPEGQuality before
	// building the PDF. Scanned/photographed book pages are almost all of
	// the final file size, so this is the main lever for a smaller PDF.
	CompressImages bool
	JPEGQuality    int // 1-100, default 75 when CompressImages is set

	SkipPDF     bool // don't build the PDF (zero value keeps the old default: build it)
	ExtractText bool // OCR every page into a single .txt file alongside (or instead of) the PDF

	// TesseractPath and TessdataDir locate a portable, bundled Tesseract
	// install (see scripts/package-windows-release.sh); ExtractText is a
	// no-op returning ErrOCRUnavailable if these aren't set to a real
	// installation.
	TesseractPath string
	TessdataDir   string
}

// Result holds the file(s) produced by Download, depending on which of
// SkipPDF/ExtractText were set.
type Result struct {
	PDFPath  string
	TextPath string
}

// Download fetches every page of the AnyFlip book at rawURL and merges
// them into a PDF inside outDir, returning the finished PDF's path.
// It respects ctx: cancelling it stops in-flight requests and PDF
// conversion, and any partial temp files are cleaned up.
func Download(ctx context.Context, rawURL, outDir string, opt Options, progress ProgressFunc) (Result, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, err
	}

	client := &http.Client{Timeout: 60 * time.Second}

	fb, err := resolveFlipbook(ctx, client, rawURL, opt.Title)
	if err != nil {
		return Result{}, err
	}

	imagesDir, err := os.MkdirTemp(outDir, ".anyflip-*")
	if err != nil {
		return Result{}, err
	}
	if !opt.KeepImages {
		defer os.RemoveAll(imagesDir)
	}

	if err := fb.downloadImages(ctx, client, imagesDir, opt, progress); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	var result Result

	if !opt.SkipPDF {
		outputPath := filepath.Join(outDir, fb.title+".pdf")
		pdfPath, err := buildPDF(ctx, outputPath, imagesDir, opt.ChunkSize, progress)
		if err != nil {
			return Result{}, err
		}
		result.PDFPath = pdfPath
	}

	if opt.ExtractText {
		textPath, err := extractText(ctx, imagesDir, outDir, fb.title, opt, progress)
		if err != nil {
			return Result{}, err
		}
		result.TextPath = textPath
	}

	return result, nil
}

var unsafeFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// sanitizeFilename strips characters that are invalid in a Windows file
// name and trims the trailing dots/spaces Windows silently drops (which
// otherwise cause "path not found" errors later).
func sanitizeFilename(name string) string {
	name = unsafeFilenameChars.ReplaceAllString(name, "")
	name = strings.Trim(strings.TrimSpace(name), ".")
	if name == "" {
		name = "anyflip-download"
	}
	return name
}
