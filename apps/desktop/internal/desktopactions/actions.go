// Package desktopactions exposes an explicit, small set of native capabilities.
// It receives only requests authenticated by desktopserver's private transport.
package desktopactions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Actions struct {
	OpenExternal      func(string) error
	OpenBrowser       func(context.Context) error
	CopyText          func(string) error
	SaveRecovery      func(context.Context) (bool, error)
	SetLanguage       func(string)
	SetTrayAppearance func(string)
	MenuStatus        func(context.Context, bool) any
	TrayActivity      func() any
	ShowMain          func(string)
	SetSleep          func(context.Context, bool) (any, error)
	Reconnect         func(context.Context) error
	Quit              func(context.Context, bool) (bool, error)
	Utilities         func() any
	SetLogin          func(bool) (any, error)
	LoginSettings     func() error
	CheckUpdates      func(context.Context) any
	Uninstall         func(context.Context) (bool, error)
}

func (a *Actions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	type request struct {
		URL      string `json:"url"`
		Text     string `json:"text"`
		Language string `json:"language"`
		Page     string `json:"page"`
		Owner    string `json:"owner"`
		Section  string `json:"section"`
		Theme    string `json:"theme"`
		Refresh  bool   `json:"refresh"`
		Enabled  *bool  `json:"enabled"`
		Full     bool   `json:"full"`
	}
	var payload request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&payload) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "invalid desktop request", http.StatusBadRequest)
		return
	}
	var err error
	var result any = map[string]any{"ok": true}
	switch r.URL.Path {
	case "/desktop/v1/open-browser":
		if payload != (request{}) {
			http.Error(w, "browser action takes no arguments", http.StatusBadRequest)
			return
		}
		err = a.OpenBrowser(r.Context())
	case "/desktop/v1/open-external":
		u, parseErr := url.Parse(payload.URL)
		if parseErr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.ContainsAny(payload.URL, "\x00\r\n") {
			http.Error(w, "unsupported link", http.StatusBadRequest)
			return
		}
		err = a.OpenExternal(payload.URL)
	case "/desktop/v1/copy-text":
		err = a.CopyText(payload.Text)
	case "/desktop/v1/save-recovery-card":
		var saved bool
		saved, err = a.SaveRecovery(r.Context())
		result = map[string]any{"saved": saved}
	case "/desktop/v1/language":
		if payload.Language != "en" && payload.Language != "zh-Hans" {
			http.Error(w, "unsupported language", http.StatusBadRequest)
			return
		}
		a.SetLanguage(payload.Language)
	case "/desktop/v1/menubar-status":
		result = a.MenuStatus(r.Context(), payload.Refresh)
	case "/desktop/v1/tray-activity":
		result = a.TrayActivity()
	case "/desktop/v1/tray-appearance":
		if payload.Theme != "light" && payload.Theme != "dark" {
			http.Error(w, "unsupported appearance", http.StatusBadRequest)
			return
		}
		a.SetTrayAppearance(payload.Theme)
	case "/desktop/v1/show-main":
		if payload.Page != "dashboard" && payload.Page != "network" && payload.Page != "diagnostics" && payload.Page != "devices" && payload.Page != "connections" {
			http.Error(w, "unsupported page", http.StatusBadRequest)
			return
		}
		if len(payload.Owner) > 256 || strings.ContainsAny(payload.Owner, "\x00\r\n") || (payload.Owner != "" && payload.Page != "connections") {
			http.Error(w, "unsupported connection owner", http.StatusBadRequest)
			return
		}
		if payload.Section != "" && (payload.Page != "dashboard" || payload.Section != "active-devices") {
			http.Error(w, "unsupported page section", http.StatusBadRequest)
			return
		}
		target := payload.Page
		if payload.Page == "connections" {
			owner := payload.Owner
			if owner == "" {
				owner = "all"
			}
			target += "?owner=" + url.QueryEscape(owner)
		}
		if payload.Section != "" {
			target += "#" + payload.Section
		}
		a.ShowMain(target)
	case "/desktop/v1/sleep-prevention":
		if payload.Enabled == nil {
			http.Error(w, "enabled is required", http.StatusBadRequest)
			return
		}
		result, err = a.SetSleep(r.Context(), *payload.Enabled)
	case "/desktop/v1/reconnect":
		err = a.Reconnect(r.Context())
	case "/desktop/v1/utilities":
		result = a.Utilities()
	case "/desktop/v1/login-item":
		if payload.Enabled == nil {
			http.Error(w, "enabled is required", http.StatusBadRequest)
			return
		}
		result, err = a.SetLogin(*payload.Enabled)
	case "/desktop/v1/login-settings":
		err = a.LoginSettings()
	case "/desktop/v1/check-updates":
		result = a.CheckUpdates(r.Context())
	case "/desktop/v1/uninstall":
		var accepted bool
		accepted, err = a.Uninstall(r.Context())
		result = map[string]bool{"accepted": accepted}
	case "/desktop/v1/quit":
		var accepted bool
		accepted, err = a.Quit(r.Context(), payload.Full)
		result = map[string]bool{"accepted": accepted}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		code := "desktop_action_failed"
		var failure *Failure
		if errors.As(err, &failure) {
			code = failure.Code
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "The desktop action could not be completed. Please try again."}})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

var ErrFailed = errors.New("desktop action failed")

// Failure carries a closed, renderer-localised error code, never command output.
type Failure struct{ Code string }

func (f *Failure) Error() string { return f.Code }
