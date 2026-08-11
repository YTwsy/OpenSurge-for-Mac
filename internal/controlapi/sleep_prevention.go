package controlapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const pmsetPath = "/usr/bin/pmset"

type SleepPreventionLease interface {
	Done() <-chan error
	Close() error
}

type SleepPreventionRunner interface {
	AcquireSleepPrevention(context.Context, string) (SleepPreventionLease, error)
}

type sleepPreventionController struct {
	mu         sync.Mutex
	runner     SleepPreventionRunner
	configPath string
	requested  bool
	changing   bool
	lease      SleepPreventionLease
	lastError  string
}

func newSleepPreventionController(runner SleepPreventionRunner, configPath string) *sleepPreventionController {
	return &sleepPreventionController{runner: runner, configPath: configPath}
}

func (c *sleepPreventionController) Status() SleepPreventionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked()
}

func (c *sleepPreventionController) SetEnabled(ctx context.Context, enabled bool) (SleepPreventionStatus, error) {
	c.mu.Lock()
	if c.changing {
		status := c.statusLocked()
		c.mu.Unlock()
		return status, fmt.Errorf("sleep prevention is already changing")
	}
	if enabled && c.lease != nil {
		status := c.statusLocked()
		c.mu.Unlock()
		return status, nil
	}
	if !enabled && c.lease == nil {
		c.requested = false
		c.lastError = ""
		status := c.statusLocked()
		c.mu.Unlock()
		return status, nil
	}
	c.changing = true
	c.requested = enabled
	lease := c.lease
	if !enabled {
		c.lease = nil
	}
	c.mu.Unlock()

	var err error
	if enabled {
		lease, err = c.runner.AcquireSleepPrevention(ctx, c.configPath)
	} else {
		err = lease.Close()
	}

	c.mu.Lock()
	c.changing = false
	if err != nil {
		c.requested = false
		c.lastError = err.Error()
	} else if enabled {
		c.lease = lease
		c.lastError = ""
		go c.observe(lease)
	} else {
		c.lastError = ""
	}
	status := c.statusLocked()
	c.mu.Unlock()
	return status, err
}

func (c *sleepPreventionController) Close() error {
	c.mu.Lock()
	c.requested = false
	lease := c.lease
	c.lease = nil
	c.mu.Unlock()
	if lease == nil {
		return nil
	}
	return lease.Close()
}

func (c *sleepPreventionController) observe(lease SleepPreventionLease) {
	err, ok := <-lease.Done()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lease != lease {
		return
	}
	c.lease = nil
	c.requested = false
	if ok && err != nil {
		c.lastError = "sleep prevention helper lease ended: " + err.Error()
	} else {
		c.lastError = "sleep prevention helper lease ended unexpectedly"
	}
}

func (c *sleepPreventionController) statusLocked() SleepPreventionStatus {
	return SleepPreventionStatus{Enabled: c.requested, Active: c.lease != nil, Error: c.lastError}
}

func (c HelperClient) AcquireSleepPrevention(ctx context.Context, configPath string) (SleepPreventionLease, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = conn.Close()
		}
	}()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewEncoder(conn).Encode(HelperRequest{Action: "sleep-prevention-hold", ConfigPath: configPath}); err != nil {
		return nil, err
	}
	var response HelperResponse
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("%s", response.Error)
	}
	_ = conn.SetDeadline(time.Time{})
	lease := &helperSleepPreventionLease{conn: conn, done: make(chan error, 1)}
	go lease.watch()
	closeOnError = false
	return lease, nil
}

type helperSleepPreventionLease struct {
	conn      net.Conn
	done      chan error
	closeOnce sync.Once
}

func (l *helperSleepPreventionLease) Done() <-chan error { return l.done }

func (l *helperSleepPreventionLease) Close() error {
	var err error
	l.closeOnce.Do(func() { err = l.conn.Close() })
	return err
}

func (l *helperSleepPreventionLease) watch() {
	buffer := make([]byte, 1)
	_, err := l.conn.Read(buffer)
	if errors.Is(err, io.EOF) {
		err = fmt.Errorf("privileged helper closed the lease")
	}
	l.done <- err
	close(l.done)
}

func (DirectRunner) AcquireSleepPrevention(ctx context.Context, _ string) (SleepPreventionLease, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("sleep prevention requires the privileged helper")
	}
	if err := setSystemSleepDisabled(ctx, true); err != nil {
		return nil, err
	}
	return &directSleepPreventionLease{done: make(chan error, 1)}, nil
}

type directSleepPreventionLease struct {
	done      chan error
	closeOnce sync.Once
}

func (l *directSleepPreventionLease) Done() <-chan error { return l.done }

func (l *directSleepPreventionLease) Close() error {
	var closeErr error
	l.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closeErr = setSystemSleepDisabled(ctx, false)
		l.done <- closeErr
		close(l.done)
	})
	return closeErr
}

func setSystemSleepDisabled(ctx context.Context, disabled bool) error {
	value := "0"
	if disabled {
		value = "1"
	}
	command := exec.CommandContext(ctx, pmsetPath, "-a", "disablesleep", value)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("set system sleep disabled=%t: %s", disabled, message)
	}
	return nil
}

func systemSleepDisabled(ctx context.Context) (bool, error) {
	command := exec.CommandContext(ctx, pmsetPath, "-g")
	output, err := command.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("read system sleep state: %w", err)
	}
	return parseSystemSleepDisabled(string(output)), nil
}

func parseSystemSleepDisabled(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "SleepDisabled") {
			return fields[len(fields)-1] == "1"
		}
	}
	return false
}
