package controlapi

import (
	"fmt"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/doctor"
)

const (
	doctorRunIdle      = "idle"
	doctorRunRunning   = "running"
	doctorRunSucceeded = "succeeded"
	doctorRunFailed    = "failed"
)

type doctorRunner func(config.Config) doctor.Report

// doctorController keeps the expensive, read-only Doctor suite away from
// request hot paths. A run is process-local and single-flight: callers observe
// the same background run until it completes, then may explicitly start one
// for the current config revision.
type doctorController struct {
	mu          sync.Mutex
	run         doctorRunner
	state       string
	revision    string
	checks      []doctor.Check
	healthy     bool
	error       string
	startedAt   *time.Time
	completedAt *time.Time
}

func newDoctorController(run doctorRunner) *doctorController {
	return &doctorController{run: run, state: doctorRunIdle, checks: []doctor.Check{}}
}

func (c *doctorController) snapshot(currentRevision string) DoctorRunStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked(currentRevision)
}

func (c *doctorController) start(cfg config.Config, revision string) (DoctorRunStatus, bool) {
	c.mu.Lock()
	if c.state == doctorRunRunning {
		status := c.snapshotLocked(revision)
		c.mu.Unlock()
		return status, false
	}
	now := time.Now().UTC()
	c.state = doctorRunRunning
	c.revision = revision
	c.checks = []doctor.Check{}
	c.healthy = false
	c.error = ""
	c.startedAt = &now
	c.completedAt = nil
	status := c.snapshotLocked(revision)
	c.mu.Unlock()

	go c.execute(cfg)
	return status, true
}

func (c *doctorController) execute(cfg config.Config) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.finish(doctor.Report{}, fmt.Errorf("Doctor panicked: %v", recovered))
		}
	}()
	c.finish(c.run(cfg), nil)
}

func (c *doctorController) finish(report doctor.Report, runErr error) {
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completedAt = &now
	if runErr != nil {
		c.state = doctorRunFailed
		c.checks = []doctor.Check{}
		c.healthy = false
		c.error = runErr.Error()
		return
	}
	c.state = doctorRunSucceeded
	c.checks = doctorChecksForControl(report.Checks)
	c.healthy = doctorHealthyForControl(c.checks)
	c.error = ""
}

func (c *doctorController) snapshotLocked(currentRevision string) DoctorRunStatus {
	current := c.revision == "" || c.revision == currentRevision
	return DoctorRunStatus{
		SchemaVersion: SchemaVersion,
		State:         c.state,
		Revision:      c.revision,
		Current:       current,
		Checks:        append([]doctor.Check{}, c.checks...),
		Healthy:       c.healthy,
		Error:         c.error,
		StartedAt:     cloneTime(c.startedAt),
		CompletedAt:   cloneTime(c.completedAt),
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
