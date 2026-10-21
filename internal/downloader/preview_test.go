package downloader

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestPreviewLive hits the real anyflip.com over the network, so it only
// runs when explicitly requested (network access isn't guaranteed in CI).
//
//	ANYFLIP_LIVE_TEST=1 go test ./internal/downloader/... -run TestPreviewLive -v
func TestPreviewLive(t *testing.T) {
	if os.Getenv("ANYFLIP_LIVE_TEST") == "" {
		t.Skip("set ANYFLIP_LIVE_TEST=1 to run this network test")
	}

	const testURL = "https://online.anyflip.com/fgoih/ieyc/mobile/index.html"

	result, err := Preview(context.Background(), testURL)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if result.Title == "" {
		t.Error("expected a non-empty title")
	}
	if result.PageCount != 150 {
		t.Errorf("expected 150 pages, got %d", result.PageCount)
	}
	if !strings.HasPrefix(result.CoverDataURL, "data:image/") {
		n := len(result.CoverDataURL)
		if n > 30 {
			n = 30
		}
		t.Errorf("expected CoverDataURL to start with %q, got %q", "data:image/", result.CoverDataURL[:n])
	}
}
