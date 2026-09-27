package main

import (
	"flag"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"open-mihomo-gateway/apps/desktop/internal/controlclient"
	"open-mihomo-gateway/apps/desktop/internal/desktopserver"
	"open-mihomo-gateway/internal/webui"
)

func main() {
	directory, err := controlclient.DefaultDirectory()
	if err != nil {
		log.Fatal("Cannot locate the OpenSurge application support directory")
	}
	flag.StringVar(&directory, "control-dir", directory, "Control Service discovery directory (use a smoke fixture for desktop acceptance)")
	flag.Parse()
	assets, err := desktopserver.New(webui.FS(), controlclient.New(directory))
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{
		Name: "OpenSurge", Description: "OpenSurge for Mac desktop host",
		Assets: application.AssetOptions{Handler: assets, DisableLogging: true},
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "OpenSurge", Width: 1280, Height: 860, MinWidth: 960, MinHeight: 640, URL: "/dashboard",
	})
	show := func() { window.Show(); window.Focus() }
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { event.Cancel(); window.Hide() })
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { show() })
	menu := app.Menu.New()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	panel := menu.AddSubmenu("控制面板 / Control Panel")
	panel.Add("显示窗口 / Show Window").SetAccelerator("CmdOrCtrl+1").OnClick(func(*application.Context) { show() })
	menu.AddRole(application.WindowMenu)
	app.Menu.Set(menu)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
