package main

import (
	"context"
	"errors"
	"time"

	"open-mihomo-gateway/apps/desktop/internal/desktopactions"
	"open-mihomo-gateway/apps/desktop/internal/native"
	"open-mihomo-gateway/apps/desktop/internal/servicelife"
	"open-mihomo-gateway/apps/desktop/internal/uninstall"
)

func (h *desktopHost) initUninstaller(installedDirectory, smoke bool) {
	availability := "preview"
	if installedDirectory && native.IsInstalledApp() {
		availability = "available"
		if uninstall.ValidateInstalledScript() != nil {
			availability = "missing"
		}
	}
	validate, run := uninstall.ValidateInstalledScript, uninstall.RunInstalled
	if smoke {
		availability = "available"
		validate = func() error { return nil }
		run = func(mode uninstall.Mode) error {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			var result struct {
				Outcome string `json:"outcome"`
			}
			if h.client.WriteJSON(ctx, "POST", "/api/v1/desktop-smoke/uninstall", map[string]string{"mode": string(mode)}, &result) != nil {
				return uninstall.ErrFailed
			}
			switch result.Outcome {
			case "success":
				return nil
			case "cancel":
				return uninstall.ErrCancelled
			default:
				return uninstall.ErrFailed
			}
		}
	}
	h.uninstaller = uninstall.New(availability, validate, h.readMenuStatus, run, h.login)
}

func (h *desktopHost) uninstall(ctx context.Context) (bool, error) {
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
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	err := h.uninstaller.Check(checkCtx)
	cancel()
	if err != nil {
		return false, uninstallError(err)
	}
	h.popup.Hide()
	selected := make(chan uninstall.Mode, 1)
	dialog := h.app.Dialog.Warning().SetTitle(h.text("卸载 OpenSurge？", "Uninstall OpenSurge?"))
	dialog.SetMessage(h.text("将移除 OpenSurge App、用户级 Control Service 与 root Helper。\n\n保留数据会保留配置、订阅、凭据、策略、运行记录和日志；彻底卸载会一并删除这些数据。\n\n系统 IPv4 forwarding 状态不会被修改。尚未完成的路由器 DHCP 与 Mac 网络恢复步骤仍需手动完成。", "Remove the OpenSurge App, user Control Service and root Helper.\n\nKeeping data preserves configuration, subscriptions, credentials, policies, runtime records and logs. Removing all data deletes them.\n\nSystem IPv4 forwarding is unchanged. Any remaining router DHCP and Mac network recovery steps must still be completed manually."))
	dialog.AddButton(h.text("保留数据并卸载", "Uninstall and Keep Data")).OnClick(func() { selected <- uninstall.KeepData })
	dialog.AddButton(h.text("彻底卸载", "Remove All Data")).OnClick(func() { selected <- uninstall.RemoveAll })
	dialog.AddButton(h.text("取消", "Cancel")).SetAsDefault().SetAsCancel().OnClick(func() { selected <- "" })
	dialog.Show()
	mode := <-selected
	if mode == "" {
		return false, nil
	}
	checkCtx, cancel = context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	err = h.uninstaller.Run(checkCtx, mode) // Rechecks status after the confirmation.
	if errors.Is(err, uninstall.ErrCancelled) {
		return false, nil
	}
	if err != nil {
		return false, uninstallError(err)
	}
	h.services.ExitUI()
	h.quitting.Store(true)
	h.app.Quit()
	return true, nil
}
func uninstallError(err error) error {
	code := "desktop_uninstall_failed"
	switch {
	case errors.Is(err, uninstall.ErrUnsafe):
		code = "desktop_uninstall_unsafe"
	case errors.Is(err, uninstall.ErrUnavailable):
		code = "desktop_uninstall_unavailable"
	case errors.Is(err, uninstall.ErrLogin):
		code = "desktop_uninstall_login"
	case errors.Is(err, uninstall.ErrRestore):
		code = "desktop_uninstall_restore"
	}
	return &desktopactions.Failure{Code: code}
}
