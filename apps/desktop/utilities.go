package main

import (
	"context"
	"time"

	"open-mihomo-gateway/apps/desktop/internal/controlclient"
	"open-mihomo-gateway/apps/desktop/internal/loginitem"
	"open-mihomo-gateway/apps/desktop/internal/native"
	"open-mihomo-gateway/apps/desktop/internal/updates"
)

// The same release tag is passed to the native and frontend builds.
var releaseTag = "v0.3.0-rc.2"

type utilitySnapshot struct {
	Login     loginitem.Snapshot `json:"login"`
	Update    updates.Snapshot   `json:"update"`
	Uninstall string             `json:"uninstall"`
}

func (h *desktopHost) utilities() any {
	return utilitySnapshot{Login: h.login.Snapshot(), Update: h.updates.Snapshot(), Uninstall: h.uninstaller.Availability()}
}
func (h *desktopHost) initUtilities(installedDirectory, smoke bool) {
	var provider loginitem.Provider
	fetch := updates.FetchLatest
	installedApp := installedDirectory && native.IsInstalledApp()
	if installedApp {
		provider = native.LoginItem{}
	}
	if smoke {
		provider = smokeLogin{h.client}
		fetch = func(ctx context.Context) (updates.Release, error) {
			var release updates.Release
			err := h.client.ReadJSON(ctx, "/api/v1/desktop-smoke/release", &release)
			return release, err
		}
	}
	h.login = loginitem.New(provider)
	h.updates = updates.New(releaseTag, fetch)
	h.loginSettings = func() error { native.OpenLoginSettings(); return nil }
	if !installedApp {
		h.loginSettings = func() error { return nil }
	}
}

// Smoke actions are isolated from the installed app's ServiceManagement state.
type smokeLogin struct{ client *controlclient.Client }

func (s smokeLogin) Status() string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var result struct {
		State string `json:"state"`
	}
	if s.client.ReadJSON(ctx, "/api/v1/desktop-smoke/login", &result) != nil {
		return "unavailable"
	}
	return result.State
}
func (s smokeLogin) SetEnabled(enabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var result map[string]any
	return s.client.WriteJSON(ctx, "PUT", "/api/v1/desktop-smoke/login", map[string]bool{"enabled": enabled}, &result)
}
