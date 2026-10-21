package controlapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/runtime"
)

// ConfigurationRollback survives the Helper boundary even when apply fails.
// A state file is only an ownership record, never evidence of gateway health.
type ConfigurationRollback struct {
	ConfigRestored       bool   `json:"config_restored"`
	RestartAttempted     bool   `json:"restart_attempted"`
	GatewayState         string `json:"gateway_state"`
	PreviousGatewayReady bool   `json:"previous_gateway_ready"`
}

type helperReloadError struct {
	message string
	phase   gateway.ReloadPhase
}

func (e *helperReloadError) Error() string { return e.message }
func (e *helperReloadError) Unwrap() error {
	return &gateway.ReloadError{Phase: e.phase, Err: errors.New(e.message)}
}

func reloadGatewayState(status gateway.Status, err error) string {
	if err != nil || status.Gateway == "" || status.RuntimeState == "" {
		return "unknown"
	}
	if status.Gateway == "stopped" && status.RuntimeState == "none" {
		return "stopped"
	}
	if status.Gateway == "running" && status.RuntimeState == "active" &&
		(status.MihomoError != "" || status.TUN == "unknown" || status.Forwarding == "unknown") {
		return "unknown"
	}
	if status.Gateway == "running" && status.RuntimeState == "active" &&
		status.MihomoError == "" && status.TUNError == "" &&
		(status.TUN == "ready" || status.TUN == "disabled") && status.IPv4Takeover == "ready" && status.IPv6Takeover != "failed" {
		return "running"
	}
	return "degraded"
}

func restorePreviousGateway(ctx context.Context, cfg config.Config, configRestored bool, reloadErr error, deps profileApplyDeps) (*ConfigurationRollback, error) {
	result := &ConfigurationRollback{ConfigRestored: configRestored, GatewayState: "unknown"}
	var failure *gateway.ReloadError
	previousPreserved := !errors.As(reloadErr, &failure) || failure.Phase != gateway.ReloadStart
	// Restoring a gateway already stopped by this transaction is cleanup, even
	// if its caller canceled. It gets a separate, bounded recovery budget.
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	var recoveryErr error
	if configRestored {
		exists, err := deps.stateExists(cfg)
		if err != nil {
			recoveryErr = fmt.Errorf("inspect gateway after failed reload: %w", err)
		} else if !exists {
			result.RestartAttempted = true
			recoveryErr = deps.start(recoveryCtx, cfg)
			previousPreserved = recoveryErr == nil
		}
	}
	statusCtx, cancelStatus := context.WithTimeout(recoveryCtx, 3*time.Second)
	defer cancelStatus()
	status, err := deps.status(statusCtx, cfg)
	result.GatewayState = reloadGatewayState(status, err)
	// A healthy residual candidate is still not the previous applied gateway.
	// Only an untouched old runtime or our successful old-config start can
	// establish restoration; retain candidate files when ownership is uncertain.
	result.PreviousGatewayReady = configRestored && previousPreserved && recoveryErr == nil && result.GatewayState == "running"
	if err != nil {
		recoveryErr = errors.Join(recoveryErr, fmt.Errorf("inspect final gateway health: %w", err))
	}
	return result, recoveryErr
}

func (s *Server) recordReloadRecoveryFailure(topology string, recoveryBefore RecoveryState, reloadErr error, rollback *ConfigurationRollback) {
	if topology != config.GatewayModeSameWiFiDHCP {
		return
	}
	recoveryBefore.Topology = topology
	recoveryBefore.Required = true
	var failure *gateway.ReloadError
	stopFailed := errors.As(reloadErr, &failure) && failure.Phase == gateway.ReloadStop
	finalState := "unknown"
	cfg, err := config.LoadRuntime(s.configPath)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		status, statusErr := s.gatewayStatus(ctx, cfg)
		cancel()
		finalState = reloadGatewayState(status, statusErr)
		// Absence also has to be confirmed from the authoritative ownership file.
		// Never offer start over partial cleanup, an unreadable or interrupted state.
		if _, exists, stateErr := runtime.LoadState(runtime.NewPaths(cfg).StateFile); stateErr != nil || exists {
			if finalState == "stopped" {
				finalState = "unknown"
			}
		}
	}
	note := "gateway reload failed; final gateway health is unconfirmed; inspect runtime before retrying or recovering the LAN"
	switch {
	case stopFailed:
		note = "gateway reload stop failed; cleanup must be completed before retrying start or recovering the LAN"
	case finalState == "running":
		note = "gateway reload failed; running gateway health confirmed; DHCP takeover stage preserved"
		if rollback != nil {
			if rollback.PreviousGatewayReady {
				note = "gateway reload failed; previous config and healthy gateway restored; DHCP takeover stage preserved"
			} else {
				note = "gateway reload failed; running gateway observed but previous applied configuration is unconfirmed; inspect applied configuration; DHCP takeover stage preserved"
			}
		}
	case finalState == "stopped":
		recoveryBefore.Stage = RecoveryRouterDHCPDisabledConfirmed
		note = "gateway reload failed after services stopped; router DHCP remains disabled; retry start or recover the LAN"
	case finalState == "degraded":
		note = "gateway reload failed; degraded runtime remains; stop and complete cleanup before retrying start or recovering the LAN"
	}
	appendRecoveryNote(&recoveryBefore, note+": "+reloadErr.Error())
	_ = s.store.SaveRecovery(recoveryBefore)
}
