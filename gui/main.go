// Command dwbt-gui is a visual editor and runner of dwbt workflows.
//
// It opens the .dwbt directory found in the directory given as the first
// argument, or the current directory, or one of their parents.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/noi/dwbt/gui/internal/studio"
	"github.com/noi/dwbt/internal/engine"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	presets, err := engine.Builtin().Map()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	dir, _ := os.Getwd()
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	app := &App{studio: studio.New(presets, os.Environ()), dir: dir}

	err = wails.Run(&options.App{
		Title:     "dwbt",
		Width:     1360,
		Height:    860,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind:      []any{app},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}
