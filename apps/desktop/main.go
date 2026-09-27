package main

import (
	"context"
	"errors"
	"html"
	"log"
	"sync/atomic"
	"time"

	"open-mihomo-gateway/apps/desktop/internal/controlclient"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The desktop executable owns windows and OS integration. The independently
// installed Control Service and Helper own gateway operations and their lifetime.
func main() {
	directory, err := controlclient.DefaultDirectory()
	if err != nil {
		log.Fatal("Cannot locate the OpenSurge application support directory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := application.New(application.Options{
		Name:        "OpenSurge",
		Description: "OpenSurge for Mac desktop host",
		OnShutdown:  cancel,
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "OpenSurge",
		Width:     1280,
		Height:    860,
		MinWidth:  960,
		MinHeight: 640,
		HTML:      connectionPage("正在连接 OpenSurge 后台服务… / Connecting to OpenSurge…"),
	})
	host := &desktopHost{ctx: ctx, client: controlclient.New(directory), window: window}
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		window.Hide()
	})
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { host.show() })
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { host.connect() })

	menu := app.Menu.New()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	panel := menu.AddSubmenu("控制面板 / Control Panel")
	panel.Add("显示窗口 / Show Window").SetAccelerator("CmdOrCtrl+1").OnClick(func(*application.Context) { host.show() })
	panel.Add("重新连接 / Reconnect").SetAccelerator("CmdOrCtrl+r").OnClick(func(*application.Context) { host.connect() })
	menu.AddRole(application.WindowMenu)
	app.Menu.Set(menu)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type desktopHost struct {
	ctx        context.Context
	client     *controlclient.Client
	window     *application.WebviewWindow
	connecting atomic.Bool
}

func (h *desktopHost) show() {
	h.window.Show()
	h.window.Focus()
}

func (h *desktopHost) connect() {
	if h.ctx.Err() != nil || !h.connecting.CompareAndSwap(false, true) {
		return
	}
	h.show()
	h.window.SetHTML(connectionPage("正在连接 OpenSurge 后台服务… / Connecting to OpenSurge…"))
	go func() {
		defer h.connecting.Store(false)
		// Bound startup retries. A retry obtains a new login grant only; it never
		// starts/stops a service or replays a frontend operation.
		for attempt := 0; ; attempt++ {
			location, err := h.client.BootstrapURL(h.ctx, "dashboard")
			if h.ctx.Err() != nil {
				return
			}
			if err == nil {
				h.window.SetURL(location)
				return
			}
			if !errors.Is(err, controlclient.ErrUnavailable) || attempt == 3 {
				h.window.SetHTML(connectionPage(err.Error()))
				return
			}
			timer := time.NewTimer(time.Second << attempt)
			select {
			case <-h.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func connectionPage(message string) string {
	return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>OpenSurge</title><style>html{color-scheme:light dark;font-family:system-ui}body{margin:0;min-height:100vh;display:grid;place-items:center}main{max-width:620px;padding:40px;line-height:1.7}h1{font-size:28px;font-weight:600}p{opacity:.75}</style><main><h1>OpenSurge</h1><p role="status">` + html.EscapeString(message) + `</p><p>请先启动已安装的 OpenSurge，然后从「控制面板」菜单选择「重新连接」（⌘R）。</p><p>Open the installed OpenSurge app, then choose Control Panel → Reconnect (⌘R).</p></main></html>`
}
