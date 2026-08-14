package controlapi

import (
	"context"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
)

const (
	mihomoRecoveryIdle       = "idle"
	mihomoRecoveryObserving  = "observing"
	mihomoRecoveryRecovering = "recovering"
	mihomoRecoveryFailed     = "failed"

	mihomoFailureProcessMissing    = "process_missing"
	mihomoFailureControllerRefused = "controller_refused"

	autoMihomoRecoveryInterval         = 5 * time.Second
	mihomoRecoveryHealthyConfirmations = 2
)

type mihomoRecoveryController struct {
	mu              sync.Mutex
	state           string
	reason          string
	error           string
	refusedCount    int
	healthyCount    int
	attempted       bool
	operationActive bool
}

func newMihomoRecoveryController() *mihomoRecoveryController {
	return &mihomoRecoveryController{state: mihomoRecoveryIdle}
}

func (c *mihomoRecoveryController) snapshot() MihomoRecoveryStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return MihomoRecoveryStatus{State: c.state, Reason: c.reason, Error: c.error}
}

func (c *mihomoRecoveryController) observeHealthy() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operationActive {
		return
	}
	if c.attempted {
		c.healthyCount++
		c.state = mihomoRecoveryRecovering
		c.error = ""
		if c.healthyCount < mihomoRecoveryHealthyConfirmations {
			return
		}
	}
	c.state = mihomoRecoveryIdle
	c.reason = ""
	c.error = ""
	c.refusedCount = 0
	c.healthyCount = 0
	c.attempted = false
}

func (c *mihomoRecoveryController) observeUnknown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operationActive {
		return
	}
	// An unreadable or inapplicable sample proves neither recovery nor
	// continued failure. Keep the one-attempt incident guard, while requiring
	// future failure and health confirmations to be consecutive again.
	c.healthyCount = 0
	c.refusedCount = 0
}

func (c *mihomoRecoveryController) observeFailure(reason string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operationActive {
		return false
	}
	c.healthyCount = 0
	if c.attempted && c.state == mihomoRecoveryRecovering {
		c.state = mihomoRecoveryFailed
		c.reason = reason
		c.error = "mihomo remained unhealthy after restart"
		return false
	}
	if c.attempted || c.state == mihomoRecoveryFailed {
		return false
	}
	if c.reason != reason {
		c.reason = reason
		c.refusedCount = 0
	}
	c.state = mihomoRecoveryObserving
	c.error = ""
	if reason == mihomoFailureProcessMissing {
		return true
	}
	if reason == mihomoFailureControllerRefused {
		c.refusedCount++
		return c.refusedCount >= 2
	}
	return false
}

func (c *mihomoRecoveryController) begin(reason string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attempted || c.state == mihomoRecoveryRecovering || c.state == mihomoRecoveryFailed {
		return false
	}
	c.state = mihomoRecoveryRecovering
	c.reason = reason
	c.error = ""
	c.attempted = true
	c.healthyCount = 0
	c.operationActive = true
	return true
}

func (c *mihomoRecoveryController) beginManual() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = mihomoRecoveryRecovering
	c.error = ""
	c.attempted = true
	c.healthyCount = 0
	c.operationActive = true
}

func (c *mihomoRecoveryController) finishAutomatic(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.operationActive = false
	if err != nil {
		c.state = mihomoRecoveryFailed
		c.error = err.Error()
		return
	}
	// The command completing is not proof that the controller is healthy. Keep
	// the incident in recovery until a fresh status observation confirms it;
	// otherwise observeFailure exposes the manual fallback without retrying.
	c.state = mihomoRecoveryRecovering
	c.error = ""
}

func (c *mihomoRecoveryController) finishManual(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.operationActive = false
	c.attempted = true
	c.refusedCount = 0
	if err != nil {
		c.state = mihomoRecoveryFailed
		c.error = err.Error()
		return
	}
	c.state = mihomoRecoveryRecovering
	c.error = ""
}

func (s *Server) monitorMihomoRecovery(ctx context.Context) {
	s.evaluateMihomoRecovery(ctx)
	ticker := time.NewTicker(autoMihomoRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluateMihomoRecovery(ctx)
		}
	}
}

func (s *Server) evaluateMihomoRecovery(ctx context.Context) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		s.mihomoRecovery.observeUnknown()
		return
	}
	status, err := s.gatewayStatus(ctx, cfg)
	if err != nil || status.RuntimeState != "active" {
		s.mihomoRecovery.observeUnknown()
		return
	}

	reason := mihomoFailureReason(status)
	if reason == "" {
		s.mihomoRecovery.observeHealthy()
		return
	}

	recovery, err := s.store.Recovery()
	if err != nil || !mihomoRecoveryStageAllowed(cfg.Gateway.Mode, recovery.Stage) {
		s.mihomoRecovery.observeUnknown()
		return
	}
	if !s.mihomoRecovery.observeFailure(reason) {
		return
	}
	if !s.lifecycleMu.TryLock() {
		return
	}
	if !s.mihomoRecovery.begin(reason) {
		s.lifecycleMu.Unlock()
		return
	}

	now := time.Now().UTC()
	op := Operation{
		SchemaVersion: SchemaVersion,
		ID:            "auto-restart-mihomo-" + randomToken(8),
		Kind:          "restart-mihomo",
		State:         "running",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.SaveOperation(op); err != nil {
		s.lifecycleMu.Unlock()
		s.mihomoRecovery.finishAutomatic(err)
		return
	}
	go s.runOperationLocked(op, cfg.Gateway.Mode, recovery, s.mihomoRecovery.finishAutomatic)
}

func mihomoFailureReason(status gateway.Status) string {
	if !strings.HasPrefix(status.Mihomo, "running") {
		return mihomoFailureProcessMissing
	}
	if connectionRefused(status.MihomoError) || connectionRefused(status.TUNError) {
		return mihomoFailureControllerRefused
	}
	return ""
}

func connectionRefused(value string) bool {
	return strings.Contains(strings.ToLower(value), "connection refused")
}

func mihomoRecoveryStageAllowed(topology, stage string) bool {
	if topology != config.GatewayModeSameWiFiDHCP {
		return true
	}
	return stage == RecoveryGatewayActive || stage == RecoveryClientValidated || stage == RecoveryClientValidationSkipped
}
