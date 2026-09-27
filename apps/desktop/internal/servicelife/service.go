// Package servicelife owns only the desktop user's existing launchd job.
// Network operations and the root Helper remain owned by the Control Service.
package servicelife

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"open-mihomo-gateway/apps/desktop/internal/menustatus"
)

var (
	ErrQuitting    = errors.New("desktop is quitting")
	ErrUnavailable = errors.New("service lifecycle is unavailable for this discovery directory")
	ErrUnsafe      = errors.New("gateway or network recovery is not stopped")
	ErrCommand     = errors.New("Control Service launchd operation failed")
)

type Runner func(context.Context, string, ...string) error
type Reader func(context.Context) (*menustatus.Status, error)

type Coordinator struct {
	mu                     sync.Mutex
	quitting               bool
	available              bool
	run                    Runner
	read                   Reader
	domain, service, agent string
}

func New(uid int, homeDirectory string, available bool, run Runner, read Reader) *Coordinator {
	domain := fmt.Sprintf("gui/%d", uid)
	return &Coordinator{available: available, run: run, read: read, domain: domain, service: domain + "/com.opensurge.control", agent: filepath.Join(homeDirectory, "Library/LaunchAgents/com.opensurge.control.plist")}
}

func Run(ctx context.Context, executable string, arguments ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Fixed native commands; output is deliberately not returned to the WebView.
	return exec.CommandContext(ctx, executable, arguments...).Run()
}

func (c *Coordinator) Available() bool { return c.available }
func (c *Coordinator) Wake(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.quitting {
		return ErrQuitting
	}
	if !c.available {
		return ErrUnavailable
	}
	if c.run(ctx, "/bin/launchctl", "print", c.service) != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if c.run(ctx, "/bin/launchctl", "bootstrap", c.domain, c.agent) != nil {
			return ErrCommand
		}
	}
	if c.run(ctx, "/bin/launchctl", "kickstart", c.service) != nil {
		return ErrCommand
	}
	return nil
}

// Stop rechecks fresh status while holding the same lock as wake, then closes
// the wake gate before bootout. It never stops the gateway or unloads the Helper.
func (c *Coordinator) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.quitting {
		return ErrQuitting
	}
	if !c.available {
		return ErrUnavailable
	}
	status, err := c.read(ctx)
	if err != nil || !CanStop(status) {
		return ErrUnsafe
	}
	c.quitting = true
	if err := c.run(ctx, "/bin/launchctl", "bootout", c.service); err != nil {
		c.quitting = false
		return ErrCommand
	}
	return nil
}
func (c *Coordinator) ExitUI() { c.mu.Lock(); c.quitting = true; c.mu.Unlock() }

func CanStop(status *menustatus.Status) bool {
	// Missing/unknown service data is not evidence that it is safe to stop.
	return status != nil && status.SchemaVersion == 1 && status.CanQuit() &&
		(status.DHCP == "stopped" || status.DHCP == "disabled") && status.Mihomo == "stopped" &&
		(status.PFAnchor == "unloaded" || status.PFAnchor == "disabled")
}
