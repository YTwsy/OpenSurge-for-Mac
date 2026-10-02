package main

import (
	"context"
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"open-mihomo-gateway/apps/desktop/internal/menustatus"
	"open-mihomo-gateway/apps/desktop/internal/native"
)

//go:embed Resources/OpenSurgeMenuBarIcon.png
var trayIcon []byte

func (h *desktopHost) showTray() {
	_ = h.tray.PositionWindow(h.popup, 8)
	h.popup.Show().Focus()
}

func (h *desktopHost) createTray() {
	h.popup = h.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "menu-bar", Title: "OpenSurge Menu Bar", URL: "/desktop-tray",
		Width: 390, Height: 550, Hidden: true, Frameless: true, DisableResize: true,
		AlwaysOnTop: true, HideOnFocusLost: true, HideOnEscape: true,
		Mac: application.MacWindow{Backdrop: application.MacBackdropTranslucent, CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces | application.MacWindowCollectionBehaviorFullScreenAuxiliary, TabbingMode: application.MacWindowTabbingModeDisallowed},
	})
	h.tray = h.app.SystemTray.New().SetTemplateIcon(trayIcon).AttachWindow(h.popup).WindowOffset(8)
	h.tray.SetTooltip("OpenSurge")
	h.popup.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) { h.status.SetRapid(true) })
	h.popup.OnWindowEvent(events.Common.WindowHide, func(*application.WindowEvent) { h.status.SetRapid(false) })
	h.popup.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { event.Cancel(); h.popup.Hide() })
}

func (h *desktopHost) readMenuStatus(ctx context.Context) (*menustatus.Status, error) {
	var status menustatus.Status
	err := h.client.ReadJSON(ctx, "/api/v1/menubar", &status)
	return &status, err
}

func (h *desktopHost) menuStatusChanged(snapshot menustatus.Snapshot) {
	h.activity.SetRunning(snapshot.Status != nil && (snapshot.Status.Gateway == "running" || snapshot.Status.Gateway == "degraded"))
	if status := snapshot.Status; status != nil {
		language := status.UIPreferences.Language
		if language == "system" {
			language = native.SystemLanguage()
		}
		if language == "en" || language == "zh-Hans" {
			h.setLanguage(language)
		}
	}
	h.mu.Lock()
	english := h.language == "en"
	h.mu.Unlock()
	descriptions := map[string][2]string{
		"connecting": {"正在连接后台服务", "Connecting to service"},
		"stopped":    {"网关已停止", "Gateway stopped"}, "running": {"网关正在运行", "Gateway running"},
		"degraded": {"网关运行异常", "Gateway needs attention"}, "recovery": {"网络恢复尚未完成", "Network recovery incomplete"},
		"unreachable":  {"无法连接后台服务", "Service unreachable"},
		"starting":     {"正在启动网关", "Starting gateway"},
		"reloading":    {"正在应用配置", "Applying configuration"},
		"stopping":     {"正在停止网关", "Stopping gateway"},
		"recovering":   {"正在恢复代理引擎", "Recovering proxy engine"},
		"rolling_back": {"正在回滚网络改动", "Rolling back network changes"},
		"changing":     {"正在更新网关", "Updating gateway"},
		"interrupted":  {"重启后待清理", "Cleanup required after reboot"},
		"unknown":      {"状态暂不可用", "Status temporarily unavailable"},
	}
	i := 0
	if english {
		i = 1
	}
	native.SetMenuBarIndicator(snapshot.Indicator, "OpenSurge — "+descriptions[snapshot.Indicator][i])
}
