package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"open-mihomo-gateway/apps/desktop/internal/controlclient"
	"open-mihomo-gateway/apps/desktop/internal/desktopactions"
	"open-mihomo-gateway/apps/desktop/internal/menustatus"
	"open-mihomo-gateway/apps/desktop/internal/native"
)

type desktopHost struct {
	app      *application.App
	main     *application.WebviewWindow
	client   *controlclient.Client
	mu       sync.Mutex
	language string
	popup    *application.WebviewWindow
	tray     *application.SystemTray
	status   *menustatus.Monitor
}

func (h *desktopHost) show(path string) {
	if h.popup != nil {
		h.popup.Hide()
	}
	h.main.UnMinimise()
	h.main.Show()
	h.main.Focus()
	if path != "" {
		value, _ := json.Marshal(path)
		h.main.ExecJS(`window.dispatchEvent(new CustomEvent('opensurge:navigate',{detail:` + string(value) + `}));`)
	}
}

func (h *desktopHost) actions() *desktopactions.Actions {
	return &desktopactions.Actions{
		OpenExternal: func(url string) error {
			if !native.ExternalURLAllowed(url) {
				return desktopactions.ErrFailed
			}
			return h.app.Browser.OpenURL(url)
		},
		CopyText: func(text string) error {
			var ok bool
			application.InvokeSync(func() { ok = h.app.Clipboard.SetText(text) })
			if !ok {
				return desktopactions.ErrFailed
			}
			return nil
		},
		SaveRecovery: h.saveRecovery,
		SetLanguage:  h.setLanguage,
		ShowMain:     h.show,
		MenuStatus: func(ctx context.Context, refresh bool) any {
			if refresh {
				return h.status.Refresh(ctx)
			}
			return h.status.Snapshot()
		},
		SetSleep: func(ctx context.Context, enabled bool) (any, error) {
			return h.status.SetSleep(ctx, func(ctx context.Context) (menustatus.SleepPrevention, error) {
				var result menustatus.SleepPrevention
				err := h.client.WriteJSON(ctx, "PUT", "/api/v1/sleep-prevention", map[string]bool{"enabled": enabled}, &result)
				return result, err
			})
		},
	}
}

func (h *desktopHost) saveRecovery(ctx context.Context) (bool, error) {
	readContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	data, err := h.client.Read(readContext, "/api/v1/recovery/card?download=1", 1<<20)
	cancel()
	if err != nil {
		return false, err
	}
	filename, err := h.app.Dialog.SaveFile().SetFilename("OpenSurge-WiFi-DHCP-Recovery-Card.txt").AddFilter("Text", "*.txt").AttachToWindow(h.main).PromptForSingleSelection()
	if err != nil || filename == "" {
		return false, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return true, os.WriteFile(filename, data, 0600)
}

func (h *desktopHost) setLanguage(language string) {
	h.mu.Lock()
	if h.language == language {
		h.mu.Unlock()
		return
	}
	h.language = language
	h.mu.Unlock()
	native.SetLanguage(h.main, language == "en")
	if h.popup != nil {
		native.SetLanguage(h.popup, language == "en")
	}
	h.setMenu(language == "en")
}

func (h *desktopHost) setMenu(english bool) {
	t := func(zh, en string) string {
		if english {
			return en
		}
		return zh
	}
	menu := h.app.Menu.New()
	appMenu := menu.AddSubmenu("OpenSurge")
	appMenu.Add(t("关于 OpenSurge", "About OpenSurge")).SetRole(application.About)
	appMenu.AddSeparator()
	appMenu.Add(t("隐藏 OpenSurge", "Hide OpenSurge")).SetRole(application.Hide).SetAccelerator("CmdOrCtrl+h")
	appMenu.Add(t("隐藏其他应用", "Hide Others")).SetRole(application.HideOthers).SetAccelerator("CmdOrCtrl+OptionOrAlt+h")
	appMenu.Add(t("显示全部", "Show All")).SetRole(application.ShowAll)
	appMenu.AddSeparator()
	appMenu.Add(t("只退出桌面 App", "Quit Desktop App Only")).SetAccelerator("CmdOrCtrl+q").OnClick(func(*application.Context) { h.app.Quit() })
	edit := menu.AddSubmenu(t("编辑", "Edit"))
	for _, item := range []struct {
		zh, en, key string
		role        application.Role
	}{
		{"撤销", "Undo", "CmdOrCtrl+z", application.Undo}, {"重做", "Redo", "CmdOrCtrl+Shift+z", application.Redo},
		{"剪切", "Cut", "CmdOrCtrl+x", application.Cut}, {"复制", "Copy", "CmdOrCtrl+c", application.Copy},
		{"粘贴", "Paste", "CmdOrCtrl+v", application.Paste}, {"全选", "Select All", "CmdOrCtrl+a", application.SelectAll},
	} {
		edit.Add(t(item.zh, item.en)).SetRole(item.role).SetAccelerator(item.key)
	}
	view := menu.AddSubmenu(t("显示", "View"))
	view.Add(t("显示主窗口", "Show Main Window")).OnClick(func(*application.Context) { h.show("") })
	view.Add(t("显示菜单栏面板", "Show Menu Bar Panel")).SetAccelerator("CmdOrCtrl+Shift+m").OnClick(func(*application.Context) { h.showTray() })
	for i, page := range []struct{ path, zh, en string }{
		{"dashboard", "总览", "Overview"}, {"network", "网络设置", "Network Settings"}, {"sources", "代理与规则源", "Sources"}, {"devices", "设备", "Devices"},
		{"connections", "连接", "Connections"}, {"policies", "策略", "Policies"}, {"connectivity", "连通性", "Connectivity"}, {"diagnostics", "诊断", "Diagnostics"},
	} {
		view.Add(t(page.zh, page.en)).SetAccelerator(fmt.Sprintf("CmdOrCtrl+%d", i+1)).OnClick(func(*application.Context) { h.show(page.path) })
	}
	view.AddSeparator()
	view.Add(t("刷新状态", "Refresh Status")).SetAccelerator("CmdOrCtrl+r").OnClick(func(*application.Context) { h.main.ExecJS(`window.dispatchEvent(new Event('opensurge:refresh'));`) })
	view.Add(t("放大", "Zoom In")).SetAccelerator("CmdOrCtrl+=").OnClick(func(*application.Context) { h.main.SetZoom(min(h.main.GetZoom()+0.1, 1.5)) })
	view.Add(t("缩小", "Zoom Out")).SetAccelerator("CmdOrCtrl+-").OnClick(func(*application.Context) { h.main.SetZoom(max(h.main.GetZoom()-0.1, 0.75)) })
	view.Add(t("实际大小", "Actual Size")).SetAccelerator("CmdOrCtrl+0").OnClick(func(*application.Context) { h.main.SetZoom(1) })
	windowMenu := menu.AddSubmenu(t("窗口", "Window"))
	windowMenu.Add(t("最小化", "Minimise")).SetRole(application.Minimise).SetAccelerator("CmdOrCtrl+m")
	windowMenu.Add(t("关闭窗口", "Close Window")).SetRole(application.CloseWindow).SetAccelerator("CmdOrCtrl+w")
	windowMenu.Add(t("进入全屏幕", "Enter Full Screen")).SetRole(application.ToggleFullscreen).SetAccelerator("CmdOrCtrl+Control+f")
	h.app.Menu.Set(menu)
}
