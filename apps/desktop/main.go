package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"open-mihomo-gateway/apps/desktop/internal/controlclient"
	"open-mihomo-gateway/apps/desktop/internal/desktopserver"
	"open-mihomo-gateway/apps/desktop/internal/menustatus"
	"open-mihomo-gateway/apps/desktop/internal/native"
	"open-mihomo-gateway/apps/desktop/internal/servicelife"
	"open-mihomo-gateway/apps/desktop/internal/trayactivity"
	"open-mihomo-gateway/internal/webui"
)

// Stamped together with Info.plist by the bundle builder. Preview and installed
// hosts must not forward launch requests to one another.
var bundleIdentifier = "com.opensurge.desktop.preview"

func main() {
	directory, err := controlclient.DefaultDirectory()
	if err != nil {
		log.Fatal("Cannot locate the OpenSurge application support directory")
	}
	defaultDirectory := directory
	smokeActions := flag.Bool("smoke-actions", false, "Use isolated fixture providers for service, login, updates and uninstall; never change installed services or login items")
	smokeStartupDelay := flag.Duration("smoke-startup-delay", 0, "Delay JavaScript assets to inspect the cold-start placeholder; requires --smoke-actions (maximum 15s)")
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
	if *smokeStartupDelay < 0 || *smokeStartupDelay > 15*time.Second || (*smokeStartupDelay > 0 && !*smokeActions) {
		log.Fatal("Startup delay requires isolated smoke actions and must be between 0 and 15s")
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
	host.activity = trayactivity.New(func(ctx context.Context) ([]byte, error) {
		return host.client.Read(ctx, "/api/v1/device-traffic", 4<<20)
	})
	host.status = menustatus.New(host.readMenuStatus, host.menuStatusChanged)
	lifetime, cancel := context.WithCancel(context.Background())
	defer cancel()
	assets, err := desktopserver.New(webui.FS(), host.client, host.actions())
	if err != nil {
		log.Fatal(err)
	}
	runtimeAssets := application.BundledAssetFileServer(webui.FS())
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *smokeStartupDelay > 0 && strings.HasSuffix(r.URL.Path, ".js") {
			select {
			case <-time.After(*smokeStartupDelay):
			case <-r.Context().Done():
				return
			}
		}
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
		Assets:                      application.AssetOptions{Handler: handler, DisableLogging: true},
		OnShutdown:                  cancel,
		ShouldQuit:                  host.shouldQuit,
		DisableDefaultSignalHandler: true,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: fmt.Sprintf("%s.%x", bundleIdentifier, sha256.Sum256([]byte(directory))),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				go func() { <-ready; host.show("") }()
			},
		},
	})
	host.app = app
	termination := make(chan os.Signal, 1)
	signal.Notify(termination, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(termination)
	go func() {
		select {
		case <-ready:
		case <-lifetime.Done():
			return
		}
		select {
		case <-termination:
			// Installer TERM stops only the host, without an interactive quit
			// dialog. The installer owns service ordering and gateway recovery.
			host.quitBusy.Store(true)
			cancel()
			host.services.ExitUI()
			host.quitting.Store(true)
			app.Quit()
		case <-lifetime.Done():
		}
	}()
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "OpenSurge", Width: 1440, Height: 900, MinWidth: 1080, MinHeight: 640, URL: "/dashboard",
		Hidden: true, BackgroundColour: application.NewRGBA(242, 247, 244, 255),
		Mac: application.MacWindow{TitleBar: application.MacTitleBarHiddenInset, InvisibleTitleBarHeight: 40},
	})
	host.main = window
	native.ConfigureMenuBarIcon()
	host.createTray()
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { event.Cancel(); window.Hide(); native.SetDockVisible(false) })
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
		host.show("")
		close(ready)
		go func() {
			// Only the installed bundle wakes the installed user job on launch.
			// Production smoke acceptance uses the same path with a fake runner.
			if (directory == defaultDirectory && native.IsInstalledApp()) || (*smokeActions && bundleIdentifier == "com.opensurge.menubar") {
				ctx, done := context.WithTimeout(lifetime, 20*time.Second)
				if err := host.reconnect(ctx); err != nil {
					log.Print("Control Service could not be woken; use Reconnect to retry")
				}
				done()
			}
			host.status.Run(lifetime)
		}()
		go host.updates.Run(lifetime)
		go host.activity.Run(lifetime)
	})
	host.setMenu(false)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
