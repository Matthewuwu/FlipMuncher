package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Matthewuwu/FlipMuncher/internal/downloader"
)

const queueUpdatedEvent = "queue-updated"

// Config is persisted to %APPDATA%/FlipMuncher/config.json (or
// the platform equivalent of os.UserConfigDir()).
type Config struct {
	OutputDir         string  `json:"outputDir"`
	Threads           int     `json:"threads"`
	Retries           int     `json:"retries"`
	RetryDelaySeconds float64 `json:"retryDelaySeconds"`
	ChunkSize         int     `json:"chunkSize"`
	KeepImages        bool    `json:"keepImages"`
	CompressImages    bool    `json:"compressImages"`
	JPEGQuality       int     `json:"jpegQuality"`
}

func defaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		OutputDir:         filepath.Join(home, "Downloads", "AnyFlip"),
		Threads:           2,
		Retries:           1,
		RetryDelaySeconds: 1,
		ChunkSize:         10,
		CompressImages:    true, // book pages are almost all of the PDF's size; on by default
		JPEGQuality:       60,   // suggested balance of size and sharpness (see the size estimate in Advanced)
	}
}

// ocrPaths locates the portable Tesseract install shipped next to the
// executable (see scripts/package-windows-release.sh). If it's missing,
// downloader.ExtractText fails with a clear ErrOCRUnavailable instead of
// silently doing nothing.
func ocrPaths() (tesseractPath, tessdataDir string) {
	exe, err := os.Executable()
	if err != nil {
		return "", ""
	}
	ocrDir := filepath.Join(filepath.Dir(exe), "ocr")
	bin := "tesseract"
	if goruntime.GOOS == "windows" {
		bin = "tesseract.exe"
	}
	return filepath.Join(ocrDir, bin), filepath.Join(ocrDir, "tessdata")
}

// QueueItem is one book in the download queue, mirrored to the frontend
// as a JSON snapshot after every change.
type QueueItem struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Status   string `json:"status"` // waiting | downloading | done | error | cancelled
	Phase    string `json:"phase"`  // downloading | converting
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Error    string `json:"error,omitempty"`
	PDFPath  string `json:"pdfPath,omitempty"`
	TextPath string `json:"textPath,omitempty"`

	titleOverride string // unexported: never sent to the frontend
	buildPDF      bool   // unexported: per-book output choice
	extractText   bool   // unexported: per-book output choice
}

const (
	statusWaiting     = "waiting"
	statusDownloading = "downloading"
	statusDone        = "done"
	statusError       = "error"
	statusCancelled   = "cancelled"
)

// App is the Wails-bound backend. The queue runs one book at a time in a
// single goroutine (ponytail: matches the spec's single "cancel current
// download" button; a concurrent-queue rewrite is unwarranted for a
// personal-use downloader).
type App struct {
	ctx context.Context

	mu     sync.Mutex
	cfg    Config
	queue  []*QueueItem
	cancel context.CancelFunc
	busy   bool
}

func NewApp() *App {
	return &App{cfg: loadConfig()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "FlipMuncher", "config.json"), nil
}

// normalizeConfig clamps settings to usable values. Missing or invalid
// fields fall back to the defaults, so a config.json written by an older
// version (without newer keys) never produces zero values.
func normalizeConfig(cfg Config) Config {
	def := defaultConfig()
	if cfg.OutputDir == "" {
		cfg.OutputDir = def.OutputDir
	}
	if cfg.Threads < 1 {
		cfg.Threads = 1
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	}
	if cfg.RetryDelaySeconds < 0 {
		cfg.RetryDelaySeconds = def.RetryDelaySeconds
	}
	if cfg.ChunkSize < 1 {
		cfg.ChunkSize = def.ChunkSize
	}
	if cfg.JPEGQuality < 1 || cfg.JPEGQuality > 100 {
		cfg.JPEGQuality = def.JPEGQuality
	}
	return cfg
}

func loadConfig() Config {
	cfg := defaultConfig()
	path, err := configPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	// Unmarshal onto the defaults so keys missing from an older file keep
	// their default instead of becoming zero.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig()
	}
	return normalizeConfig(cfg)
}

// GetConfig returns the current settings for the Advanced panel.
func (a *App) GetConfig() Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// SaveConfig persists settings and remembers the last-used output folder.
func (a *App) SaveConfig(cfg Config) error {
	cfg = normalizeConfig(cfg)
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return nil
}

// PickOutputFolder opens a native folder picker starting at the current
// output folder.
func (a *App) PickOutputFolder() (string, error) {
	a.mu.Lock()
	start := a.cfg.OutputDir
	a.mu.Unlock()

	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Choose output folder",
		DefaultDirectory: start,
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// PreviewBook fetches a book's title, page count, and first page (as a
// data: URL) without downloading the rest of it or writing any files.
func (a *App) PreviewBook(url string) (downloader.PreviewResult, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return downloader.Preview(ctx, url)
}

// folderOpener returns the command that opens a folder in the platform's
// file manager.
func folderOpener(goos, path string) (name string, args []string) {
	switch goos {
	case "windows":
		return "explorer", []string{path}
	case "darwin":
		return "open", []string{path}
	default:
		return "xdg-open", []string{path}
	}
}

// OpenFolder opens the given folder in the system file manager.
func (a *App) OpenFolder(path string) error {
	name, args := folderOpener(goruntime.GOOS, path)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// explorer.exe often exits 1 even on success, so don't check the exit
	// code; Wait just reaps the process.
	go cmd.Wait()
	return nil
}

// snapshot returns a JSON-safe copy of the queue for the frontend.
func (a *App) snapshot() []QueueItem {
	out := make([]QueueItem, len(a.queue))
	for i, item := range a.queue {
		out[i] = *item
	}
	return out
}

// emitQueue sends the current queue to the frontend. It takes the lock
// itself, so callers must not hold a.mu.
func (a *App) emitQueue() {
	a.mu.Lock()
	snap := a.snapshot()
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, queueUpdatedEvent, snap)
}

// QueueRequest is one book to add to the queue: its URL, the title from
// its preview (editable by the user before adding), and the output
// choice made on its preview card.
type QueueRequest struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	BuildPDF    bool   `json:"buildPDF"`
	ExtractText bool   `json:"extractText"`
}

// normalizeQueueRequest forces BuildPDF on when both outputs are off, so
// a queued book always produces something (mirrors the rule SaveConfig
// used to enforce globally). Pure function — no App state — so it can be
// unit tested directly without a running Wails app.
func normalizeQueueRequest(r QueueRequest) QueueRequest {
	if !r.BuildPDF && !r.ExtractText {
		r.BuildPDF = true
	}
	return r
}

// enqueue appends the given books and reports whether the caller must
// start the worker. Marking the worker busy under the same lock that reads
// it makes sure concurrent StartQueue calls start exactly one worker.
func (a *App) enqueue(items []QueueRequest) (startWorker bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, req := range items {
		if req.URL == "" {
			continue
		}
		req = normalizeQueueRequest(req)
		item := &QueueItem{
			ID:            fmt.Sprintf("%d", time.Now().UnixNano()+int64(len(a.queue))),
			URL:           req.URL,
			Status:        statusWaiting,
			titleOverride: req.Title,
			buildPDF:      req.BuildPDF,
			extractText:   req.ExtractText,
		}
		a.queue = append(a.queue, item)
	}
	if a.busy {
		return false
	}
	a.busy = true
	return true
}

// StartQueue adds the given books to the queue and, if nothing is
// already running, starts processing it. Each book carries its own
// title (from its preview, possibly edited) and output choice (from its
// preview card's toggles).
func (a *App) StartQueue(items []QueueRequest) {
	start := a.enqueue(items)
	a.emitQueue()
	if start {
		go a.runQueue()
	}
}

// claimNext takes the next waiting book and marks it downloading, in one
// critical section, so it can never be handed out twice. When nothing is
// waiting it clears busy in that same section, so a book queued right
// afterwards starts a new worker instead of being stranded.
func (a *App) claimNext() (item *QueueItem, cfg Config, ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, it := range a.queue {
		if it.Status == statusWaiting {
			it.Status = statusDownloading
			c, cancel := context.WithCancel(a.ctx)
			a.cancel = cancel
			return it, a.cfg, c
		}
	}
	a.busy = false
	return nil, Config{}, nil
}

func (a *App) runQueue() {
	for {
		item, cfg, ctx := a.claimNext()
		if item == nil {
			a.emitQueue()
			return
		}
		a.emitQueue()

		tesseractPath, tessdataDir := ocrPaths()
		opt := downloader.Options{
			Title:          item.titleOverride,
			Threads:        cfg.Threads,
			Retries:        cfg.Retries,
			RetryDelay:     time.Duration(cfg.RetryDelaySeconds * float64(time.Second)),
			ChunkSize:      cfg.ChunkSize,
			KeepImages:     cfg.KeepImages,
			CompressImages: cfg.CompressImages,
			JPEGQuality:    cfg.JPEGQuality,
			SkipPDF:        !item.buildPDF,
			ExtractText:    item.extractText,
			TesseractPath:  tesseractPath,
			TessdataDir:    tessdataDir,
		}

		result, err := downloader.Download(ctx, item.URL, cfg.OutputDir, opt, func(p downloader.Progress) {
			a.mu.Lock()
			item.Phase = p.Phase
			item.Done = p.Done
			item.Total = p.Total
			a.mu.Unlock()
			a.emitQueue()
		})

		a.mu.Lock()
		if a.cancel != nil {
			a.cancel() // release the per-book context
			a.cancel = nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				item.Status = statusCancelled
			} else {
				item.Status = statusError
				item.Error = friendlyError(err)
			}
		} else {
			item.Status = statusDone
			item.PDFPath = result.PDFPath
			item.TextPath = result.TextPath
		}
		a.mu.Unlock()
		a.emitQueue()
	}
}

// CancelCurrent stops whichever book is downloading right now.
func (a *App) CancelCurrent() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func friendlyError(err error) string {
	switch {
	case errors.Is(err, downloader.ErrInvalidURL):
		return "That doesn't look like an AnyFlip book URL. Use a link like https://online.anyflip.com/xxxx/yyyy/ or its mobile/index.html page."
	case errors.Is(err, downloader.ErrOCRUnavailable):
		return "Text extraction isn't available: the 'ocr' folder is missing next to the app. Re-download the full portable release, or turn off \"Extract text\"."
	case errors.Is(err, os.ErrPermission):
		return "Permission denied writing to the output folder. Choose another folder."
	case errors.Is(err, context.DeadlineExceeded):
		return "The request timed out. Check your connection and try again."
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "Network error: could not reach anyflip.com. Check your connection and try again."
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return "Network error: could not reach anyflip.com. Check your connection and try again."
	}
	return err.Error()
}

// AboutInfo backs the About dialog.
type AboutInfo struct {
	Version        string `json:"version"`
	UpstreamURL    string `json:"upstreamUrl"`
	License        string `json:"license"`
	LicenseText    string `json:"licenseText"`
	UpstreamNotice string `json:"upstreamNotice"`
}

// GetAboutInfo returns the credits, license text and rights notice shown
// in the About dialog (GPLv3 §5(d) appropriate legal notice).
func (a *App) GetAboutInfo() AboutInfo {
	return AboutInfo{
		Version:     appVersion,
		UpstreamURL: "https://github.com/Lofter1/anyflip-downloader",
		License:     "GNU General Public License v3.0",
		LicenseText: licenseText,
		UpstreamNotice: "FlipMuncher comes with ABSOLUTELY NO WARRANTY. " +
			"This is free software, and you are welcome to redistribute it under the terms of the GNU GPL v3. " +
			"Only download books you have the right to download.",
	}
}
