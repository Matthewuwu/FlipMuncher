package downloader

import (
	"context"
	"encoding/base64"
	"net/http"
	"time"
)

// PreviewResult is a lightweight look at a book before it's queued: its
// title, page count, and the first page as an inline image. No temp
// files or directories are created.
type PreviewResult struct {
	Title        string `json:"title"`
	PageCount    int    `json:"pageCount"`
	CoverDataURL string `json:"coverDataUrl"`
}

// Preview resolves rawURL's config.js (title, page count) and fetches
// only its first page image, returning it as a data: URL.
func Preview(ctx context.Context, rawURL string) (PreviewResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	fb, err := resolveFlipbook(ctx, client, rawURL, "")
	if err != nil {
		return PreviewResult{}, err
	}
	if len(fb.pageURLs) == 0 {
		return PreviewResult{}, ErrInvalidURL
	}

	body, err := fetchPageBytes(ctx, client, fb.pageURLs[0], fb.baseURL.String(), 1, time.Second)
	if err != nil {
		return PreviewResult{}, err
	}

	contentType := http.DetectContentType(body)
	dataURL := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(body)

	return PreviewResult{
		Title:        fb.title,
		PageCount:    fb.pageCount,
		CoverDataURL: dataURL,
	}, nil
}
