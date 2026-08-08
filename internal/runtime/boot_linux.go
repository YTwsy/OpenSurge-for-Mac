//go:build linux

package runtime

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func CurrentBootSession() (BootSession, error) {
	id, idErr := os.ReadFile("/proc/sys/kernel/random/boot_id")
	stat, statErr := os.Open("/proc/stat")
	boot := BootSession{ID: strings.TrimSpace(string(id))}
	if statErr == nil {
		defer stat.Close()
		scanner := bufio.NewScanner(stat)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) != 2 || fields[0] != "btime" {
				continue
			}
			seconds, err := strconv.ParseInt(fields[1], 10, 64)
			if err == nil {
				boot.StartedAt = time.Unix(seconds, 0)
			}
			break
		}
	}
	if boot.ID == "" && boot.StartedAt.IsZero() {
		return BootSession{}, fmt.Errorf("read Linux boot session: id: %v; boot time: %v", idErr, statErr)
	}
	return boot, nil
}
