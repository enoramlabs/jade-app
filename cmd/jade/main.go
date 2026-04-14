//go:build !unit

package main

import (
	"embed"
	"flag"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// --vault <path> opens a specific vault at launch (used by OpenInNewWindow).
	vaultFlag := flag.String("vault", "", "Open a specific vault directory at launch")
	flag.Parse()

	app := NewApp()

	// If a vault path was passed on the command line, pre-set it so that
	// initFromConfig (called from startup) will open it instead of the last-used vault.
	if *vaultFlag != "" {
		app.startupVaultOverride = *vaultFlag
	}

	err := wails.Run(&options.App{
		Title:  "jade",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
