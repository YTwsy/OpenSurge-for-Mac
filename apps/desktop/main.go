package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"open-mihomo-gateway/apps/desktop/internal/controlclient"
	"open-mihomo-gateway/apps/desktop/internal/desktopserver"
	"open-mihomo-gateway/apps/desktop/internal/native"
	"open-mihomo-gateway/internal/webui"
)

func main() {
	directory, err := controlclient.DefaultDirectory()
	if err != nil {
		log.Fatal("Cannot locate the OpenSurge application support directory")
	}
	flag.StringVar(&directory, "control-dir", directory, "Control Service discovery directory (use a smoke fixture for desktop acceptance)")
	flag.Parse()
	directory, err = filepath.Abs(directory)
	if err != nil {
		log.Fatal("Invalid Control Service directory")
	}
	ready := make(chan struct{})
	host := &desktopHost{client: controlclient.New(directory)}
	assets, err := desktopserver.New(webui.FS(), host.client, host.actions())
	if err != nil {
		log.Fatal(err)
	}
	runtimeAssets := application.BundledAssetFileServer(webui.FS())
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wails/runtime.js" {
			runtimeAssets.ServeHTTP(w, r)
			return
		}
		assets.ServeHTTP(w, r)
	})
	app := application.New(application.Options{
		Name: "OpenSurge", Description: "OpenSurge for Mac desktop host",
		Assets: application.AssetOptions{Handler: handler, DisableLogging: true},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: fmt.Sprintf("com.opensurge.desktop.preview.%x", sha256.Sum256([]byte(directory))),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				go func() { <-ready; host.show("") }()
			},
		},
	})
	host.app = app
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "OpenSurge", Width: 1280, Height: 860, MinWidth: 1080, MinHeight: 640, URL: "/dashboard",
	})
	host.main = window
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { event.Cancel(); window.Hide() })
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { host.show("") })
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		native.Configure(window, true)
		host.mu.Lock()
		language := host.language
		host.mu.Unlock()
		if language != "" {
			native.SetLanguage(window, language == "en")
		}
		close(ready)
	})
	host.setMenu(false)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
