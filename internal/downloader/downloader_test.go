package downloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDownloadLive hits the real anyflip.com over the network, so it only
// runs when explicitly requested (network access isn't guaranteed in CI).
//
//	ANYFLIP_LIVE_TEST=1 go test ./internal/downloader/... -run TestDownloadLive -v
func TestDownloadLive(t *testing.T) {
	if os.Getenv("ANYFLIP_LIVE_TEST") == "" {
		t.Skip("set ANYFLIP_LIVE_TEST=1 to run this network test")
	}

	const testURL = "https://online.anyflip.com/fgoih/ieyc/mobile/index.html"
	dir := t.TempDir()

	var lastProgress Progress
	result, err := Download(context.Background(), testURL, dir, Options{Threads: 8}, func(p Progress) {
		lastProgress = p
	})
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if filepath.Base(result.PDFPath) != "Zobeck Gazetteer.pdf" {
		t.Errorf("expected PDF named %q, got %q", "Zobeck Gazetteer.pdf", filepath.Base(result.PDFPath))
	}
	if lastProgress.Total != 150 {
		t.Errorf("expected 150 pages, got %d", lastProgress.Total)
	}
	if info, err := os.Stat(result.PDFPath); err != nil || info.Size() == 0 {
		t.Errorf("expected a non-empty PDF at %s, stat err=%v", result.PDFPath, err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("temp image dir %s was not cleaned up", e.Name())
		}
	}
}

// TestDownloadCompression checks that CompressImages produces a
// meaningfully smaller PDF than the uncompressed download.
func TestDownloadCompression(t *testing.T) {
	if os.Getenv("ANYFLIP_LIVE_TEST") == "" {
		t.Skip("set ANYFLIP_LIVE_TEST=1 to run this network test")
	}

	const testURL = "https://online.anyflip.com/fgoih/ieyc/mobile/index.html"

	plainDir := t.TempDir()
	plainResult, err := Download(context.Background(), testURL, plainDir, Options{Threads: 8}, nil)
	if err != nil {
		t.Fatalf("uncompressed Download failed: %v", err)
	}
	plainInfo, err := os.Stat(plainResult.PDFPath)
	if err != nil {
		t.Fatalf("stat uncompressed PDF: %v", err)
	}

	compressedDir := t.TempDir()
	compressedResult, err := Download(context.Background(), testURL, compressedDir, Options{Threads: 8, CompressImages: true, JPEGQuality: 60}, nil)
	if err != nil {
		t.Fatalf("compressed Download failed: %v", err)
	}
	compressedInfo, err := os.Stat(compressedResult.PDFPath)
	if err != nil {
		t.Fatalf("stat compressed PDF: %v", err)
	}

	t.Logf("uncompressed: %d bytes, compressed: %d bytes", plainInfo.Size(), compressedInfo.Size())
	if compressedInfo.Size() >= plainInfo.Size() {
		t.Errorf("expected compression to shrink the PDF: uncompressed=%d compressed=%d", plainInfo.Size(), compressedInfo.Size())
	}
}

// TestDownloadCancellation checks that cancelling the context stops the
// download promptly and cleans up the temp image directory, without
// hitting the network for more than the first page.
func TestDownloadCancellation(t *testing.T) {
	if os.Getenv("ANYFLIP_LIVE_TEST") == "" {
		t.Skip("set ANYFLIP_LIVE_TEST=1 to run this network test")
	}

	const testURL = "https://online.anyflip.com/fgoih/ieyc/mobile/index.html"
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_, err := Download(ctx, testURL, dir, Options{Threads: 1}, func(p Progress) {
			cancel()
		})
		if err == nil {
			t.Error("expected an error after cancellation")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("cancellation did not stop the download in time")
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("temp image dir %s was not cleaned up after cancellation", e.Name())
		}
	}
}
