package mihomo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/process"
)

// StartupError distinguishes an exhausted readiness budget from cancellation,
// an exited process, and an explicit listener failure. The last probe error is
// retained without interpreting connection refused as the cause of slow startup.
type StartupError struct {
	Stage   string
	Reason  string
	Elapsed time.Duration
	Err     error
}

func (e *StartupError) Error() string {
	return fmt.Sprintf("mihomo %s readiness %s after %s: %v", e.Stage, e.Reason, e.Elapsed.Round(time.Millisecond), e.Err)
}

func (e *StartupError) Unwrap() error { return e.Err }

// A done probe is either ready or an explicit, non-retryable startup failure.
// Both each probe and the polling interval share the enclosing stage deadline.
func (m Manager) waitForReadiness(ctx context.Context, pid int, stage string, timeout, probeTimeout time.Duration, probe func(context.Context) (done bool, err error)) error {
	started := time.Now()
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	failure := func(reason string, err error) error {
		// Preserve a bounded explicit startup diagnostic before a transaction
		// restarts the previous engine and replaces the current log.
		if data, readErr := os.ReadFile(m.paths.MihomoLog); readErr == nil {
			var detail string
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "level=fatal") || strings.Contains(line, "Start TUN listening error:") || strings.Contains(line, "Listener "+config.IPv6PacketListenerName+" listen err:") {
					detail = line
				}
			}
			if detail != "" {
				if len(detail) > 2048 {
					detail = detail[:2048]
				}
				err = fmt.Errorf("%w; startup log: %s", err, detail)
			}
		}
		return &StartupError{Stage: stage, Reason: reason, Elapsed: time.Since(started), Err: err}
	}
	contextFailure := func() error {
		reason := "timed out"
		if errors.Is(readyCtx.Err(), context.Canceled) {
			reason = "canceled"
		}
		return failure(reason, errors.Join(readyCtx.Err(), lastErr))
	}
	for {
		if readyCtx.Err() != nil {
			return contextFailure()
		}
		if !process.IsAlive(pid) {
			return failure("process exited", fmt.Errorf("pid %d exited before readiness; last probe: %v", pid, lastErr))
		}
		probeCtx, cancelProbe := context.WithTimeout(readyCtx, probeTimeout)
		done, err := probe(probeCtx)
		cancelProbe()
		lastErr = err
		if readyCtx.Err() != nil {
			return contextFailure()
		}
		if done {
			if err != nil {
				return failure("failed", err)
			}
			if !process.IsAlive(pid) {
				return failure("process exited", fmt.Errorf("pid %d exited during readiness probe", pid))
			}
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-readyCtx.Done():
			timer.Stop()
			return contextFailure()
		case <-timer.C:
		}
	}
}
