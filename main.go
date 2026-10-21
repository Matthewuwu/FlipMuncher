package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed LICENSE
var licenseText string

// appVersion is set at build time from the release tag (-ldflags -X main.appVersion=...).
var appVersion = "0.0.0-dev"

// webview2RuntimeDir returns the absolute path of the "webview2runtime"
// folder shipped next to the executable (see build/package-release.sh).
// If it doesn't exist, it returns "" so Wails uses a system-installed WebView2.
func webview2RuntimeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Join(filepath.Dir(exe), "webview2runtime")
	// Wails errors out on a missing path instead of falling back.
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	return dir
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "FlipMuncher",
		Width:     900,
		Height:    700,
		MinWidth:  520,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 26, B: 32, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewBrowserPath: webview2RuntimeDir(),
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
