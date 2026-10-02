package native

/*
#cgo LDFLAGS: -framework ServiceManagement
#include <stdbool.h>
#include <stdlib.h>
const char *openSurgeLoginStatus(void);
bool setOpenSurgeLogin(bool enabled, char **message);
void openSurgeLoginSettings(void);
bool openSurgeIsInstalledApp(void);
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type LoginItem struct{}

func (LoginItem) Status() string {
	var state string
	application.InvokeSync(func() { state = C.GoString(C.openSurgeLoginStatus()) })
	return state
}
func (LoginItem) SetEnabled(enabled bool) error {
	var ok bool
	var message *C.char
	application.InvokeSync(func() { ok = bool(C.setOpenSurgeLogin(C.bool(enabled), &message)) })
	if message != nil {
		defer C.free(unsafe.Pointer(message))
	}
	if !ok {
		if message != nil {
			return errors.New(C.GoString(message))
		}
		return errors.New("login item update failed")
	}
	return nil
}
func OpenLoginSettings()   { application.InvokeSync(func() { C.openSurgeLoginSettings() }) }
func IsInstalledApp() bool { return bool(C.openSurgeIsInstalledApp()) }
