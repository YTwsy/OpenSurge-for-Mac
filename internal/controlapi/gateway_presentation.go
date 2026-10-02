package controlapi

import (
	"context"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/gatewayview"
)

// Only operations owned by this service instance are live. Persisted operations
// from an interrupted instance remain diagnostic history, never a busy lease.
type gatewayActivity struct {
	mu         sync.Mutex
	generation uint64
	active     map[string]Operation
}

func (a *gatewayActivity) update(op Operation) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		a.active = make(map[string]Operation)
	}
	_, existed := a.active[op.ID]
	if op.State != "running" {
		if existed {
			delete(a.active, op.ID)
			a.generation++
		}
		return
	}
	if !existed {
		a.generation++
	}
	// Snapshot only immutable display fields; do not retain a shared Notices slice.
	op.Notices = nil
	a.active[op.ID] = op
}

type gatewayActivitySnapshot struct {
	generation uint64
	known      bool
	operation  *Operation
}

func (a *gatewayActivity) snapshot() gatewayActivitySnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := gatewayActivitySnapshot{generation: a.generation, known: len(a.active) > 0}
	for _, op := range a.active {
		if operationGatewayState(op) != "" && (result.operation == nil || op.CreatedAt.After(result.operation.CreatedAt)) {
			copy := op
			result.operation = &copy
		}
	}
	return result
}

func operationGatewayState(op Operation) string {
	var state string
	switch op.Kind {
	case "start":
		state = "starting"
	case "reload":
		state = "reloading"
	case "stop":
		state = "stopping"
	case "restart-mihomo":
		state = "recovering"
	case "apply-profile", "apply-tailscale":
		state = "changing"
	default:
		return ""
	}
	if op.Phase == "rolling_back" || op.Phase == "restoring_config" {
		return "rolling_back"
	}
	return state
}

// Read across a lifecycle boundary at most twice. If an operation completes
// during the component probes, its partial sample must not become a new fault.
func (s *Server) observeGateway(ctx context.Context, cfg config.Config) (gateway.Status, error, RecoveryState, gatewayview.Status) {
	for attempt := 0; ; attempt++ {
		before := s.gatewayActivity.snapshot()
		busyBefore, lockErr := gateway.LifecycleOperationInProgress(cfg)
		status, statusErr := s.gatewayStatus(ctx, cfg)
		recovery, _ := s.store.Recovery()
		after := s.gatewayActivity.snapshot()
		busyAfter, afterErr := gateway.LifecycleOperationInProgress(cfg)
		changed := before.generation != after.generation || busyBefore != busyAfter
		if changed && attempt == 0 && ctx.Err() == nil {
			continue
		}
		view := deriveGatewayPresentation(status, statusErr, recovery, after, busyAfter, time.Now())
		if changed || (lockErr != nil || afterErr != nil) && status.Gateway == "degraded" {
			view.State, view.Reason = "unknown", "status_refreshing"
		}
		return status, statusErr, recovery, view
	}
}

func deriveGatewayPresentation(status gateway.Status, statusErr error, recovery RecoveryState, activity gatewayActivitySnapshot, externalBusy bool, now time.Time) gatewayview.Status {
	view := gatewayview.Status{State: status.Gateway, Busy: activity.known || externalBusy}
	if op := activity.operation; op != nil {
		view.State, view.Phase, view.OperationID = operationGatewayState(*op), op.Phase, op.ID
		if now.Sub(op.CreatedAt) >= gatewayOperationTimeout {
			view.State, view.Reason = "unknown", "operation_unconfirmed"
		}
		return view
	}
	if externalBusy && !activity.known {
		view.State, view.Reason = "changing", "external_operation"
		return view
	}
	if statusErr != nil || status.Gateway == "" {
		view.State, view.Reason = "unknown", "status_unavailable"
		return view
	}
	if status.RuntimeState == "interrupted" {
		view.State, view.Reason = "interrupted", "runtime_interrupted"
		return view
	}
	if recoveryNeedsAttention(recovery) {
		view.State, view.Reason = "recovery", recovery.Stage
		return view
	}
	if status.Gateway == "stopped" {
		return view
	}
	// Concrete component failures are distinct from failed observations and
	// desired configuration/Doctor warnings. PF and forwarding count as well.
	switch {
	case status.DHCP == "stopped":
		view.Reason = "dns_stopped"
	case status.Mihomo == "stopped":
		view.Reason = "mihomo_stopped"
	case status.TUN == "failed" || status.TUN == "stopped":
		view.Reason = "tun_failed"
	case status.IPv6Packet == "failed":
		view.Reason = "ipv6_failed"
	case status.PFAnchor == "unloaded":
		view.Reason = "pf_unloaded"
	case status.Forwarding == "disabled":
		view.Reason = "forwarding_disabled"
	}
	if view.Reason != "" {
		view.State = "degraded"
		return view
	}
	if status.TUN == "unknown" || status.Forwarding == "unknown" || strings.TrimSpace(status.MihomoError) != "" {
		view.State, view.Reason = "unknown", "status_unavailable"
		return view
	}
	if status.Gateway == "degraded" || status.IPv4Takeover == "failed" || status.IPv6Takeover == "failed" {
		view.State, view.Reason = "degraded", "takeover_failed"
	}
	return view
}

func recoveryNeedsAttention(recovery RecoveryState) bool {
	if !recovery.Required {
		return false
	}
	switch recovery.Stage {
	case RecoveryIdle, RecoveryPrepared, RecoveryGatewayActive, RecoveryClientValidated, RecoveryClientValidationSkipped, RecoveryComplete, RecoveryCompleteStatic:
		return false
	}
	return true
}
