package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"open-mihomo-gateway/apps/desktop/internal/native"
)

func (h *desktopHost) createSettings() {
	h.settings = h.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "settings", Title: "OpenSurge 设置", URL: "/desktop-settings",
		Width: 560, Height: 540, Hidden: true, DisableResize: true,
		MinimiseButtonState: application.ButtonDisabled, MaximiseButtonState: application.ButtonDisabled,
		FullscreenButtonState: application.ButtonDisabled,
		BackgroundColour:      application.NewRGBA(242, 247, 244, 255),
		Mac:                   application.MacWindow{TabbingMode: application.MacWindowTabbingModeDisallowed},
	})
	h.settings.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		h.settings.Hide()
		native.RefreshDockVisibility()
	})
}

func (h *desktopHost) showSettings() {
	if h.popup != nil {
		h.popup.Hide()
	}
	h.settings.Show()
	native.Present(h.settings)
}
