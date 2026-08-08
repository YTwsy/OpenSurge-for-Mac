//go:build darwin

package runtime

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"time"
)

func CurrentBootSession() (BootSession, error) {
	idValue, idErr := syscall.Sysctl("kern.bootsessionuuid")
	bootValue, bootErr := syscall.Sysctl("kern.boottime")
	boot := BootSession{ID: strings.Trim(strings.TrimSpace(idValue), "\x00")}
	if bootErr == nil {
		boot.StartedAt = parseDarwinBootTimeValue(bootValue)
	}
	if boot.ID == "" && boot.StartedAt.IsZero() {
		return BootSession{}, fmt.Errorf("read macOS boot session: uuid: %v; boot time: %v", idErr, bootErr)
	}
	return boot, nil
}

func parseDarwinBootTimeValue(value string) time.Time {
	if len(value) >= 8 {
		seconds := int64(binary.LittleEndian.Uint64([]byte(value[:8])))
		if seconds > 0 {
			return time.Unix(seconds, 0)
		}
	}
	return parseDarwinBootTime(value)
}
