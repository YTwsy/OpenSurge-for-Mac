package main

import (
	"context"
	"errors"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"open-mihomo-gateway/apps/desktop/internal/menustatus"
	"open-mihomo-gateway/apps/desktop/internal/native"
	"open-mihomo-gateway/apps/desktop/internal/servicelife"
	"open-mihomo-gateway/apps/desktop/internal/uninstall"
)

type desktopSnapshot struct {
	menustatus.Snapshot
	ServiceActions bool `json:"service_actions"`
	CanUninstall   bool `json:"can_uninstall"`
}

func (h *desktopHost) menuSnapshot(ctx context.Context, refresh bool) any {
	snapshot := h.status.Snapshot()
	if refresh {
		snapshot = h.status.Refresh(ctx)
	}
	snapshot.CanQuit = servicelife.CanStop(snapshot.Status) && !h.quitBusy.Load()
	return desktopSnapshot{Snapshot: snapshot, ServiceActions: h.services.Available(), CanUninstall: uninstall.CanUninstall(snapshot.Status) && !h.quitBusy.Load()}
}
func (h *desktopHost) text(zh, en string) string {
	h.mu.Lock()
	english := h.language == "en"
	h.mu.Unlock()
	if english {
		return en
	}
	return zh
}
func (h *desktopHost) reconnect(ctx context.Context) error {
	if !h.quitBusy.CompareAndSwap(false, true) {
		return servicelife.ErrQuitting
	}
	defer h.quitBusy.Store(false)
	if err := h.services.Wake(ctx); err != nil {
		return err
	}
	h.status.Refresh(ctx)
	h.main.ExecJS(`window.dispatchEvent(new Event('opensurge:refresh'));`)
	return nil
}

func (h *desktopHost) foregroundWarning(title, message string) (*application.MessageDialog, func()) {
	visible, minimised := h.main.IsVisible(), h.main.IsMinimised()
	h.show("")
	restore := func() {
		if minimised {
			h.main.Minimise()
		} else if !visible {
			h.main.Hide()
			native.SetDockVisible(false)
		}
	}
	return h.app.Dialog.Warning().SetTitle(title).SetMessage(message).AttachToWindow(h.main), restore
}

func (h *desktopHost) confirm(title, message, accept string) bool {
	result := make(chan bool, 1)
	dialog, restore := h.foregroundWarning(title, message)
	dialog.AddButton(accept).OnClick(func() { result <- true })
	dialog.AddButton(h.text("取消", "Cancel")).SetAsCancel().SetAsDefault().OnClick(func() { result <- false })
	dialog.Show()
	accepted := <-result
	if !accepted {
		restore()
	}
	return accepted
}
func (h *desktopHost) quit(ctx context.Context, full bool) (bool, error) {
	if !h.quitBusy.CompareAndSwap(false, true) {
		return false, servicelife.ErrQuitting
	}
	wasVisible := h.popup.IsVisible()
	defer func() {
		h.quitBusy.Store(false)
		if wasVisible && !h.quitting.Load() {
			h.showTray()
		}
	}()
	readCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	status, err := h.readMenuStatus(readCtx)
	cancel()
	if err != nil {
		status = nil
	}
	title := h.text("只退出桌面 App？", "Quit Desktop App Only?")
	message := h.text("只关闭主窗口和菜单栏，后台控制服务会继续运行。", "Close the main window and menu bar. The background Control Service will continue running.")
	if status == nil {
		message = h.text("当前无法确认网关状态。只关闭主窗口和菜单栏，不会停止后台控制服务或网关。", "Gateway status is unavailable. Only the main window and menu bar will close; the Control Service and gateway will not be stopped.")
	} else if status.ServicesActive() {
		message = h.text("网关仍在运行。只关闭主窗口和菜单栏，DHCP/DNS、mihomo、PF/转发和后台控制服务都会继续运行；此操作也不会完成网络恢复。", "The gateway is running. Only the main window and menu bar will close. DHCP/DNS, mihomo, PF/forwarding and the Control Service will keep running; this does not complete network recovery.")
	}
	if full {
		if !h.services.Available() || !servicelife.CanStop(status) {
			return false, servicelife.ErrUnsafe
		}
		title = h.text("退出 OpenSurge？", "Quit OpenSurge?")
		message = h.text("网关数据面已经停止。将停止用户级 Control Service 并退出桌面 App；root Helper 保持空闲加载，下次打开时可重新连接。", "The gateway data plane has stopped. Stop the user Control Service and quit the desktop App. The root Helper remains loaded and idle; reconnect when you next open the App.")
	}
	if !h.confirm(title, message, h.text("确认退出", "Quit")) {
		return false, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if full {
		// Confirmations may stay open while another client changes the gateway.
		// Coordinator.Stop revalidates immediately before the fixed launchd action.
		stopCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := h.services.Stop(stopCtx); err != nil {
			return false, err
		}
	} else {
		h.services.ExitUI()
	}
	h.quitting.Store(true)
	h.app.Quit()
	return true, nil
}
func (h *desktopHost) quitFromMenu(full bool) {
	go func() {
		_, err := h.quit(context.Background(), full)
		if errors.Is(err, servicelife.ErrQuitting) {
			return
		}
		if err != nil && !h.quitting.Load() {
			dialog, restore := h.foregroundWarning("OpenSurge", h.text("未能退出。请重新连接后台服务，并确认已在网络设置中停止网关、完成恢复。", "Could not quit. Reconnect to the service, stop the gateway and complete recovery in Network Settings."))
			dialog.AddButton(h.text("好", "OK")).SetAsDefault().OnClick(restore)
			dialog.Show()
		}
	}()
}
func (h *desktopHost) shouldQuit() bool {
	if h.quitting.Load() {
		return true
	}
	// AppKit's termination callback runs on the main thread. Read status and show
	// confirmation asynchronously so neither HTTP nor the native event loop blocks.
	h.quitFromMenu(false)
	return false
}

func (h *desktopHost) trayMenu(english bool) *application.Menu {
	text := func(zh, en string) string {
		if english {
			return en
		}
		return zh
	}
	menu := h.app.Menu.New()
	menu.Add(text("打开 OpenSurge 面板", "Open OpenSurge Dashboard")).OnClick(func(*application.Context) { h.show("dashboard") })
	menu.Add(text("网络设置", "Network Settings")).OnClick(func(*application.Context) { h.show("network") })
	menu.AddSeparator()
	menu.Add(text("退出 OpenSurge…", "Quit OpenSurge…")).OnClick(func(*application.Context) { h.quitFromMenu(true) })
	menu.Add(text("只退出桌面 App…", "Quit Desktop App Only…")).OnClick(func(*application.Context) { h.quitFromMenu(false) })
	return menu
}
