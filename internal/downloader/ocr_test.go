package downloader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// writeFakeTesseract creates a stand-in for tesseract.exe that just prints
// the image filename it was asked to OCR, so extractText's own logic
// (page ordering, concurrency, error propagation) can be tested without a
// real Tesseract install or a Windows machine.
func writeFakeTesseract(t *testing.T, dir string, failOn string) string {
	t.Helper()
	script := filepath.Join(dir, "fake-tesseract.sh")
	body := `#!/bin/sh
base=$(basename "$1")
if [ "$base" = "` + failOn + `" ]; then
  echo "boom" >&2
  exit 1
fi
echo "TEXT-OF-$base"
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestExtractTextOrdersPagesAndUsesTesseract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake tesseract is a shell script, doesn't apply on windows")
	}

	imagesDir := t.TempDir()
	for _, name := range []string{"0002.jpg", "0000.jpg", "0001.jpg"} {
		if err := os.WriteFile(filepath.Join(imagesDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	outDir := t.TempDir()
	fake := writeFakeTesseract(t, t.TempDir(), "")

	var calls atomic.Int32
	textPath, err := extractText(context.Background(), imagesDir, outDir, "MyBook", Options{TesseractPath: fake, Threads: 3}, func(p Progress) {
		calls.Add(1)
	})
	if err != nil {
		t.Fatalf("extractText failed: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 progress callbacks, got %d", n)
	}

	data, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatalf("reading output text: %v", err)
	}
	got := string(data)
	wantOrder := []string{"TEXT-OF-0000.jpg", "TEXT-OF-0001.jpg", "TEXT-OF-0002.jpg"}
	lastIdx := -1
	for _, w := range wantOrder {
		idx := strings.Index(got, w)
		if idx == -1 {
			t.Fatalf("expected output to contain %q, got:\n%s", w, got)
		}
		if idx < lastIdx {
			t.Fatalf("pages out of order in output:\n%s", got)
		}
		lastIdx = idx
	}
}

func TestExtractTextPropagatesPerPageFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake tesseract is a shell script, doesn't apply on windows")
	}

	imagesDir := t.TempDir()
	for _, name := range []string{"0000.jpg", "0001.jpg"} {
		if err := os.WriteFile(filepath.Join(imagesDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fake := writeFakeTesseract(t, t.TempDir(), "0001.jpg")

	_, err := extractText(context.Background(), imagesDir, t.TempDir(), "MyBook", Options{TesseractPath: fake, Threads: 2}, nil)
	if err == nil {
		t.Fatal("expected an error when one page fails OCR")
	}
}

func TestExtractTextRequiresTesseractPath(t *testing.T) {
	_, err := extractText(context.Background(), t.TempDir(), t.TempDir(), "MyBook", Options{}, nil)
	if !errors.Is(err, ErrOCRUnavailable) {
		t.Errorf("expected ErrOCRUnavailable, got %v", err)
	}

	_, err = extractText(context.Background(), t.TempDir(), t.TempDir(), "MyBook", Options{TesseractPath: "/no/such/binary"}, nil)
	if !errors.Is(err, ErrOCRUnavailable) {
		t.Errorf("expected ErrOCRUnavailable for a missing path, got %v", err)
	}
}
