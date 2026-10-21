package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestBuildPDFCancelled(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "0000.png"), []byte("x"), 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buildPDF(ctx, filepath.Join(dir, "o.pdf"), dir, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestGetBookTitleSpecialChars(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`bookConfig.bookTitle="A: B = C";`, "A: B = C"},
		{`{"title":"Vol 2: The End"}`, "Vol 2: The End"},
	} {
		if got, err := getBookTitle(c.in); err != nil || got != c.want {
			t.Errorf("%q: got %q, %v", c.in, got, err)
		}
	}
}

func TestUnsupportedExtErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	fb := &flipbook{baseURL: u, pageCount: 1, pageURLs: []string{srv.URL + "/1.gif"}}
	err := fb.downloadImages(context.Background(), srv.Client(), t.TempDir(), Options{Threads: 1}, nil)
	if err == nil {
		t.Fatal("expected error for .gif")
	}
}

func TestFirstErrorStopsSiblings(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "no", 500)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	fb := &flipbook{baseURL: u, pageCount: 200}
	for i := 0; i < 200; i++ {
		fb.pageURLs = append(fb.pageURLs, srv.URL+"/1.jpg")
	}
	if err := fb.downloadImages(context.Background(), srv.Client(), t.TempDir(), Options{Threads: 2}, nil); err == nil {
		t.Fatal("expected error")
	}
	if n := hits.Load(); n > 20 {
		t.Fatalf("kept fetching after error: %d requests", n)
	}
}

func TestPrependPath(t *testing.T) {
	if got := prependPath("/d", ""); got != "/d" {
		t.Errorf("got %q", got)
	}
	if got := prependPath("/d", "/x"); got != "/d:/x" {
		t.Errorf("got %q", got)
	}
}
