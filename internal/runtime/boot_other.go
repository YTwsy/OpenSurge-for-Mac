//go:build !darwin && !linux

package runtime

import (
	"fmt"
	goruntime "runtime"
)

func CurrentBootSession() (BootSession, error) {
	return BootSession{}, fmt.Errorf("boot session detection is unsupported on %s", goruntime.GOOS)
}
