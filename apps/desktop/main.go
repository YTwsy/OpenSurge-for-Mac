package main

import (
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The desktop executable owns windows and OS integration. The independently
// installed Control Service and Helper own gateway operations and their lifetime.
func main() {
	app := application.New(application.Options{
		Name:        "OpenSurge",
		Description: "OpenSurge for Mac desktop host",
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "OpenSurge",
		Width:     1280,
		Height:    860,
		MinWidth:  960,
		MinHeight: 640,
		HTML:      `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>OpenSurge</title><style>html{color-scheme:light dark;font-family:system-ui}body{margin:0;min-height:100vh;display:grid;place-items:center}h1{font-size:28px;font-weight:600}</style><h1>OpenSurge</h1></html>`,
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
