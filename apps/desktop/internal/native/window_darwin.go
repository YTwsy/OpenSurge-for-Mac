// Package native contains the small AppKit/WKWebView adaptations not exposed by
// the pinned Wails host API. All gateway actions remain in the Control Service.
package native

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include <stdbool.h>
void configureOpenSurgeWindow(void *window, bool rememberFrame);
void setOpenSurgeWindowLanguage(void *window, bool english);
bool openSurgeExternalURLAllowed(const char *url);
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func Configure(window *application.WebviewWindow, rememberFrame bool) {
	application.InvokeSync(func() { C.configureOpenSurgeWindow(window.NativeWindow(), C.bool(rememberFrame)) })
}

func SetLanguage(window *application.WebviewWindow, english bool) {
	application.InvokeSync(func() { C.setOpenSurgeWindowLanguage(window.NativeWindow(), C.bool(english)) })
}

func ExternalURLAllowed(value string) bool {
	url := C.CString(value)
	defer C.free(unsafe.Pointer(url))
	return bool(C.openSurgeExternalURLAllowed(url))
}
