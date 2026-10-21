// Derived from anyflip-downloader (github.com/Lofter1/anyflip-downloader),
// Copyright (C) Lofter1 and contributors, licensed under the GNU General
// Public License v3.0. Modified 2026 for use as an importable package by
// the AnyFlip-EasyDownload project (also GPL-3.0): removed package-level
// mutable state, added context cancellation, bounded worker pool safety,
// atomic PDF writes, and URL validation. See LICENSE-anyflip-downloader
// for the original license text.

package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrInvalidURL is returned when the given URL does not look like an
// AnyFlip book URL (wrong host, or missing the book path segments that
// the original tool's sanitizeURL would otherwise panic on).
var ErrInvalidURL = errors.New("not a valid anyflip.com book URL")

type flipbook struct {
	baseURL   *url.URL
	title     string
	pageCount int
	pageURLs  []string
}

// resolveFlipbook validates the URL, fetches config.js and builds the
// page URL list. It never mutates package state, so it is safe to call
// concurrently for different books.
func resolveFlipbook(ctx context.Context, client *http.Client, rawURL, titleOverride string) (*flipbook, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if !strings.Contains(u.Host, "anyflip.com") || len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return nil, ErrInvalidURL
	}
	u.Path = path.Join("/", segments[0], segments[1])

	fb := &flipbook{baseURL: u}

	configjs, err := downloadConfigJS(ctx, client, u)
	if err != nil {
		return nil, err
	}

	title := titleOverride
	if title == "" {
		title, err = getBookTitle(configjs)
		if err != nil {
			title = path.Base(u.Path)
		}
	}
	fb.title = sanitizeFilename(title)

	fb.pageCount, err = getPageCount(configjs)
	if err != nil {
		return nil, fmt.Errorf("could not determine page count: %w", err)
	}

	pageFileNames := getPageFileNames(configjs)
	downloadURL, _ := url.Parse("https://online.anyflip.com/")
	if len(pageFileNames) < fb.pageCount {
		for i := 1; i <= fb.pageCount; i++ {
			downloadURL.Path = path.Join(u.Path, "files", "mobile", strconv.Itoa(i)+".jpg")
			fb.pageURLs = append(fb.pageURLs, downloadURL.String())
		}
	} else {
		for i := 0; i < fb.pageCount; i++ {
			downloadURL.Path = path.Join(u.Path, "files", "large", pageFileNames[i])
			fb.pageURLs = append(fb.pageURLs, cleanDownloadURL(downloadURL.String()))
		}
	}

	return fb, nil
}

func (fb *flipbook) downloadImages(ctx context.Context, client *http.Client, dir string, opt Options, progress ProgressFunc) error {
	threads := opt.Threads
	if threads < 1 {
		threads = 1
	}

	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	total := fb.pageCount
	pages := make(chan int)
	results := make(chan error)

	go func() {
		defer close(pages)
		for p := 0; p < fb.pageCount; p++ {
			select {
			case pages <- p:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(threads)
	for i := 0; i < threads; i++ {
		go func() {
			defer wg.Done()
			for p := range pages {
				results <- fb.downloadPage(ctx, client, p, dir, opt)
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	completed := 0
	var firstErr error
	for err := range results {
		if err != nil && firstErr == nil {
			firstErr = err
			cancel() // stop sibling downloads
		}
		completed++
		if progress != nil {
			progress(Progress{Phase: PhaseDownloading, Done: completed, Total: total})
		}
	}

	if firstErr != nil {
		return firstErr
	}
	return parent.Err()
}

func (fb *flipbook) downloadPage(ctx context.Context, client *http.Client, page int, dir string, opt Options) error {
	pageURL := fb.pageURLs[page]

	retries := opt.Retries
	if retries < 0 {
		retries = 0
	}
	retryDelay := opt.RetryDelay
	if retryDelay <= 0 {
		retryDelay = time.Second
	}

	body, err := fetchPageBytes(ctx, client, pageURL, fb.baseURL.String(), retries, retryDelay)
	if err != nil {
		return fmt.Errorf("download failed for page %d: %w", page+1, err)
	}

	ext := strings.ToLower(path.Ext(pageURL))
	if opt.CompressImages {
		if recompressed, ok := recompressToJPEG(body, opt.JPEGQuality); ok {
			body = recompressed
			ext = ".jpg"
		}
	}

	if !supportedImageExt(ext) {
		return fmt.Errorf("page %d: unsupported image type %q", page+1, ext)
	}
	filename := fmt.Sprintf("%04d%s", page, ext)
	return os.WriteFile(filepath.Join(dir, filename), body, 0o644)
}

// fetchPageBytes fetches pageURL with retries, returning the response
// body. Shared by downloadPage (which writes the result to disk) and
// Preview (which returns just the first page's bytes as a data: URL).
// Note: this collapses the old "server returned %s" (bad status) and
// "download failed" (transport error) messages into one wrapped error
// at the caller — no test in this project asserts the exact string.
func fetchPageBytes(ctx context.Context, client *http.Client, pageURL, referer string, retries int, retryDelay time.Duration) ([]byte, error) {
	var resp *http.Response
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Referer", referer)
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err = client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// cleanDownloadURL removes problematic sequences from the download URL
// and resolves any "../" path traversal by traversing up the directory
// structure. Some books' config.js filenames contain escaped "../"
// segments (e.g. "..\/files\/mobile\/1.webp") that must be normalized
// before path.Clean can collapse them correctly.
func cleanDownloadURL(rawURL string) string {
	decoded, err := url.PathUnescape(rawURL)
	if err != nil {
		decoded = rawURL
	}
	decoded = strings.ReplaceAll(decoded, "\\", "/")

	u, err := url.Parse(decoded)
	if err != nil {
		return decoded
	}

	u.Path = path.Clean(u.Path)

	segments := strings.Split(u.Path, "/")
	deduped := segments[:0]
	for i, seg := range segments {
		if i > 0 && seg == segments[i-1] && seg != "" {
			continue
		}
		deduped = append(deduped, seg)
	}
	u.Path = strings.Join(deduped, "/")

	return u.String()
}
