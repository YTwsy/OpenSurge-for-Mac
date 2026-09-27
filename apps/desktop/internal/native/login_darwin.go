package native

/*
#cgo LDFLAGS: -framework ServiceManagement
#include <stdbool.h>
int openSurgeLoginStatus(void);
bool setOpenSurgeLogin(bool enabled);
void openSurgeLoginSettings(void);
*/
import "C"

import (
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type LoginItem struct{}

func (LoginItem) Status() string {
	var value int
	application.InvokeSync(func() { value = int(C.openSurgeLoginStatus()) })
	switch value {
	case 0:
		return "disabled"
	case 1:
		return "enabled"
	case 2:
		return "approval"
	default:
		return "unavailable"
	}
}
func (LoginItem) SetEnabled(enabled bool) error {
	var ok bool
	application.InvokeSync(func() { ok = bool(C.setOpenSurgeLogin(C.bool(enabled))) })
	if !ok {
		return errors.New("login item update failed")
	}
	return nil
}
func OpenLoginSettings() { application.InvokeSync(func() { C.openSurgeLoginSettings() }) }
