package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if cliDispatch() {
		return
	}

	// Plan §6: detect WebView2 before anything else; explicit user guidance,
	// never silent install.
	ensureWebView2OrExit()

	app, err := NewApp()
	if err != nil {
		log.Fatalf("VideoDelite startup failed: %v", err)
	}

	err = wails.Run(&options.App{
		Title:            "VideoDelite",
		Width:            1180,
		Height:           780,
		MinWidth:         980,
		MinHeight:        640,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 245, G: 245, B: 247, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
			Theme:                windows.SystemDefault,
			// Keep the WebView2 cache inside our own data tree
			// (%LOCALAPPDATA%/VideoDelite) instead of Roaming.
			WebviewUserDataPath: webviewDataPath(),
		},
	})
	if err != nil {
		log.Fatalf("VideoDelite runtime error: %v", err)
	}
}
