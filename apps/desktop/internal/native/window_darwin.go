// Package native contains the small AppKit/WKWebView adaptations not exposed by
// the pinned Wails host API. All gateway actions remain in the Control Service.
package native

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include <stdbool.h>
void configureOpenSurgeWindow(void *window, bool rememberFrame, bool tray);
void setOpenSurgeWindowLanguage(void *window, bool english);
bool openSurgeExternalURLAllowed(const char *url);
bool openSurgeSystemUsesEnglish(void);
void setOpenSurgeWindowAppearance(void *window, int preference);
void configureOpenSurgeMenuBarIcon(void);
void setOpenSurgeMenuBarIndicator(const char *indicator, const char *description);
void setOpenSurgeDockVisible(bool visible);
void refreshOpenSurgeDockVisibility(void);
void presentOpenSurgeWindow(void *window);
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func Configure(window *application.WebviewWindow, rememberFrame bool) {
	application.InvokeSync(func() {
		C.configureOpenSurgeWindow(window.NativeWindow(), C.bool(rememberFrame), C.bool(!rememberFrame))
	})
}

func ConfigureSettings(window *application.WebviewWindow) {
	application.InvokeSync(func() { C.configureOpenSurgeWindow(window.NativeWindow(), false, false) })
}

func RefreshDockVisibility() {
	application.InvokeSync(func() { C.refreshOpenSurgeDockVisibility() })
}

func SetLanguage(window *application.WebviewWindow, english bool) {
	application.InvokeSync(func() { C.setOpenSurgeWindowLanguage(window.NativeWindow(), C.bool(english)) })
}

func ExternalURLAllowed(value string) bool {
	url := C.CString(value)
	defer C.free(unsafe.Pointer(url))
	return bool(C.openSurgeExternalURLAllowed(url))
}

func SystemLanguage() string {
	if bool(C.openSurgeSystemUsesEnglish()) {
		return "en"
	}
	return "zh-Hans"
}

// Apply the popup/settings preference, including clearing overrides for System.
func SetAppearance(window *application.WebviewWindow, theme string) {
	preference := C.int(0)
	if theme == "light" {
		preference = 1
	} else if theme == "dark" {
		preference = 2
	}
	application.InvokeSync(func() { C.setOpenSurgeWindowAppearance(window.NativeWindow(), preference) })
}

func ConfigureMenuBarIcon() {
	C.configureOpenSurgeMenuBarIcon()
}

func SetMenuBarIndicator(indicator, description string) {
	value := C.CString(indicator)
	defer C.free(unsafe.Pointer(value))
	label := C.CString(description)
	defer C.free(unsafe.Pointer(label))
	application.InvokeSync(func() { C.setOpenSurgeMenuBarIndicator(value, label) })
}

func SetDockVisible(visible bool) {
	application.InvokeSync(func() { C.setOpenSurgeDockVisible(C.bool(visible)) })
}

// Present is only for explicit launch, reopen and user-triggered dialogs. It does
// not make the main window permanently floating or reactivate background polls.
func Present(window *application.WebviewWindow) {
	application.InvokeSync(func() { C.presentOpenSurgeWindow(window.NativeWindow()) })
}
