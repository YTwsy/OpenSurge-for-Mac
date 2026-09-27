package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"open-mihomo-gateway/apps/desktop/internal/controlclient"
	"open-mihomo-gateway/apps/desktop/internal/desktopserver"
	"open-mihomo-gateway/apps/desktop/internal/menustatus"
	"open-mihomo-gateway/apps/desktop/internal/native"
	"open-mihomo-gateway/apps/desktop/internal/servicelife"
	"open-mihomo-gateway/internal/webui"
)

func main() {
	directory, err := controlclient.DefaultDirectory()
	if err != nil {
		log.Fatal("Cannot locate the OpenSurge application support directory")
	}
	defaultDirectory := directory
	smokeActions := flag.Bool("smoke-actions", false, "Use isolated fixture providers for service, login, updates and uninstall; never change installed services or login items")
	flag.StringVar(&directory, "control-dir", directory, "Control Service discovery directory (use a smoke fixture for desktop acceptance)")
	flag.Parse()
	directory, err = filepath.Abs(directory)
	if err != nil {
		log.Fatal("Invalid Control Service directory")
	}
	ready := make(chan struct{})
	host := &desktopHost{client: controlclient.New(directory)}
	if *smokeActions && directory == defaultDirectory {
		log.Fatal("Smoke actions require an isolated --control-dir")
	}
	runner := servicelife.Run
	if *smokeActions {
		runner = func(ctx context.Context, executable string, arguments ...string) error {
			var result map[string]any
			return host.client.WriteJSON(ctx, "POST", "/api/v1/desktop-smoke/lifecycle", map[string]any{"executable": executable, "arguments": arguments}, &result)
		}
	}
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Cannot locate the user LaunchAgent")
	}
	host.services = servicelife.New(os.Getuid(), homeDirectory, directory == defaultDirectory || *smokeActions, runner, host.readMenuStatus)
	host.initUtilities(directory == defaultDirectory, *smokeActions)
	host.initUninstaller(directory == defaultDirectory, *smokeActions)
	host.status = menustatus.New(host.readMenuStatus, host.menuStatusChanged)
	lifetime, cancel := context.WithCancel(context.Background())
	defer cancel()
	assets, err := desktopserver.New(webui.FS(), host.client, host.actions())
	if err != nil {
		log.Fatal(err)
	}
	runtimeAssets := application.BundledAssetFileServer(webui.FS())
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if host.quitBusy.Load() && strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "A desktop lifecycle action is pending", http.StatusConflict)
			return
		}
		if r.URL.Path == "/wails/runtime.js" {
			runtimeAssets.ServeHTTP(w, r)
			return
		}
		assets.ServeHTTP(w, r)
	})
	app := application.New(application.Options{
		Name: "OpenSurge", Description: "OpenSurge for Mac desktop host",
		Assets:     application.AssetOptions{Handler: handler, DisableLogging: true},
		OnShutdown: cancel,
		ShouldQuit: host.shouldQuit,
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
	host.createTray()
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { event.Cancel(); window.Hide() })
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { host.show("") })
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		native.Configure(window, true)
		native.Configure(host.popup, false)
		host.mu.Lock()
		language := host.language
		host.mu.Unlock()
		if language != "" {
			native.SetLanguage(window, language == "en")
		}
		close(ready)
		go host.status.Run(lifetime)
		go host.updates.Run(lifetime)
	})
	host.setMenu(false)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
