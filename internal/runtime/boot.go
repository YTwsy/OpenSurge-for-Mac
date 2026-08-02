package runtime

import (
	"strconv"
	"strings"
	"time"
)

type BootSession struct {
	ID        string
	StartedAt time.Time
}

func (s State) BelongsToBoot(boot BootSession) bool {
	stateID := strings.TrimSpace(s.BootSessionID)
	bootID := strings.TrimSpace(boot.ID)
	if stateID != "" && bootID != "" {
		return strings.EqualFold(stateID, bootID)
	}
	if s.StartedAt.IsZero() || boot.StartedAt.IsZero() {
		return false
	}
	return !s.StartedAt.Before(boot.StartedAt)
}

func parseDarwinBootTime(value string) time.Time {
	marker := "sec ="
	index := strings.Index(value, marker)
	if index < 0 {
		return time.Time{}
	}
	rest := strings.TrimSpace(value[index+len(marker):])
	end := strings.IndexAny(rest, ", }")
	if end >= 0 {
		rest = rest[:end]
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}
