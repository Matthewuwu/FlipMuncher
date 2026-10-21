package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNormalizeQueueRequestForcesAnOutput(t *testing.T) {
	req := normalizeQueueRequest(QueueRequest{URL: "https://online.anyflip.com/a/b/", BuildPDF: false, ExtractText: false})
	if !req.BuildPDF {
		t.Error("expected BuildPDF to be forced true when both outputs are off")
	}
	if req.ExtractText {
		t.Error("expected ExtractText to stay false")
	}
}

func TestNormalizeQueueRequestLeavesExplicitChoiceAlone(t *testing.T) {
	req := normalizeQueueRequest(QueueRequest{URL: "https://online.anyflip.com/a/b/", BuildPDF: false, ExtractText: true})
	if req.BuildPDF {
		t.Error("expected BuildPDF to stay false when ExtractText is already on")
	}
	if !req.ExtractText {
		t.Error("expected ExtractText to stay true")
	}
}

func TestEnqueueStartsExactlyOneWorker(t *testing.T) {
	a := &App{}
	var wg sync.WaitGroup
	var started int32
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.enqueue([]QueueRequest{{URL: "https://online.anyflip.com/a/b/"}}) {
				atomic.AddInt32(&started, 1)
			}
		}()
	}
	wg.Wait()
	if started != 1 {
		t.Fatalf("expected exactly one worker to start, got %d", started)
	}
	if len(a.queue) != 50 {
		t.Fatalf("expected 50 queued books, got %d", len(a.queue))
	}
}

func TestClaimNextHandsOutEachBookOnceThenReleasesWorker(t *testing.T) {
	a := &App{ctx: context.Background()}
	if !a.enqueue([]QueueRequest{{URL: "u1"}, {URL: "u2"}}) {
		t.Fatal("first enqueue must start a worker")
	}
	first, _, _ := a.claimNext()
	second, _, _ := a.claimNext()
	if first == nil || second == nil || first == second {
		t.Fatalf("expected two distinct books, got %v and %v", first, second)
	}
	if first.Status != statusDownloading {
		t.Errorf("claimed book must be marked downloading, got %q", first.Status)
	}
	if third, _, _ := a.claimNext(); third != nil {
		t.Fatal("no third book should be waiting")
	}
	if a.busy {
		t.Error("busy must be cleared when the queue runs dry")
	}
	// A book added right after the worker stopped must start a new worker.
	if !a.enqueue([]QueueRequest{{URL: "u3"}}) {
		t.Error("enqueue after the worker stopped must request a new worker")
	}
}

func TestFolderOpener(t *testing.T) {
	cases := map[string]string{"windows": "explorer", "darwin": "open", "linux": "xdg-open", "freebsd": "xdg-open"}
	for goos, want := range cases {
		name, args := folderOpener(goos, "/x/y")
		if name != want || len(args) != 1 || args[0] != "/x/y" {
			t.Errorf("%s: got %s %v, want %s [/x/y]", goos, name, args, want)
		}
	}
}

func TestNormalizeConfigFillsInvalidValues(t *testing.T) {
	got := normalizeConfig(Config{})
	def := defaultConfig()
	if got.OutputDir != def.OutputDir || got.JPEGQuality != def.JPEGQuality || got.ChunkSize != def.ChunkSize {
		t.Errorf("zero config should fall back to defaults, got %+v", got)
	}
	if got.Threads != 1 {
		t.Errorf("threads must be at least 1, got %d", got.Threads)
	}
	if q := normalizeConfig(Config{JPEGQuality: 250}).JPEGQuality; q != def.JPEGQuality {
		t.Errorf("out-of-range quality should reset to %d, got %d", def.JPEGQuality, q)
	}
}

func TestLoadConfigKeepsDefaultsForMissingKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	path, err := configPath()
	if err != nil {
		t.Skip("no user config dir on this platform")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// An older config.json: no jpegQuality, compressImages or outputDir.
	if err := os.WriteFile(path, []byte(`{"threads": 4}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig()
	def := defaultConfig()
	if cfg.Threads != 4 {
		t.Errorf("saved threads should be kept, got %d", cfg.Threads)
	}
	if cfg.JPEGQuality != def.JPEGQuality || !cfg.CompressImages || cfg.OutputDir != def.OutputDir {
		t.Errorf("missing keys should keep defaults, got %+v", cfg)
	}

	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig(); got != def {
		t.Errorf("corrupt file should give defaults, got %+v", got)
	}
}
