package controlapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func (s *Server) handleTailscale(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	revision := fileDigest(s.configPath)
	if r.Method == http.MethodGet {
		w.Header().Set("ETag", `"`+revision+`"`)
		writeJSON(w, http.StatusOK, tailscaleResponseFrom(cfg, revision))
		return
	}
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != revision {
		writeError(w, http.StatusConflict, "revision_conflict", "If-Match must contain the current config revision")
		return
	}
	var input TailscaleUpdateRequest
	if err := decodeJSON(r, &input, 256<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	_, gatewayActive, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "runtime_state_invalid", err.Error())
		return
	}
	if gatewayActive {
		if !s.lifecycleMu.TryLock() {
			writeError(w, http.StatusConflict, "operation_in_progress", "another gateway lifecycle operation is already running")
			return
		}
		defer s.lifecycleMu.Unlock()
		if input.Enabled && input.AcceptRoutes {
			if conflicts := s.tailscaleSubnetRouteConflicts(r.Context(), manualTailscaleSubnetRouteCandidates(input.SubnetRoutes)); len(conflicts) > 0 {
				writeError(w, http.StatusConflict, "tailscale_route_conflict", tailscaleRouteConflictMessage(conflicts[0]))
				return
			}
		}
	}
	payload, _ := json.Marshal(input)
	ctx, operation, ok := s.beginRequestOperation(w, r, "apply-tailscale")
	if !ok {
		return
	}
	result, err := s.configRunner.ApplyTailscale(ctx, s.configPath, match, payload)
	if err == nil && gatewayActive && !result.Reloaded {
		err = fmt.Errorf("running gateway did not reload the Tailscale configuration")
	}
	s.finishOperation(operation, err)
	if err != nil {
		status, code := http.StatusUnprocessableEntity, "tailscale_validation_failed"
		switch {
		case strings.Contains(err.Error(), "revision conflict"):
			status, code = http.StatusConflict, "revision_conflict"
		case strings.Contains(err.Error(), "operation"):
			status, code = http.StatusConflict, "operation_in_progress"
		case strings.Contains(err.Error(), "reload failed"):
			status, code = http.StatusConflict, "tailscale_reload_failed"
		}
		writeError(w, status, code, err.Error())
		return
	}
	cfg, err = config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_reload_failed", err.Error())
		return
	}
	w.Header().Set("ETag", `"`+result.Revision+`"`)
	writeJSON(w, http.StatusOK, tailscaleResponseFrom(cfg, result.Revision))
}

func (s *Server) handleTailscaleForgetIdentity(w http.ResponseWriter, r *http.Request) {
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != fileDigest(s.configPath) {
		writeError(w, http.StatusConflict, "revision_conflict", "If-Match must contain the current config revision")
		return
	}
	if !s.lifecycleMu.TryLock() {
		writeError(w, http.StatusConflict, "operation_in_progress", "another gateway lifecycle operation is already running")
		return
	}
	defer s.lifecycleMu.Unlock()
	revision, err := s.configRunner.ForgetTailscaleIdentity(r.Context(), s.configPath, match)
	if err != nil {
		status, code := http.StatusUnprocessableEntity, "tailscale_identity_forget_failed"
		if strings.Contains(err.Error(), "must be stopped") || strings.Contains(err.Error(), "disable Tailscale") {
			status, code = http.StatusConflict, "tailscale_identity_in_use"
		}
		writeError(w, status, code, err.Error())
		return
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_reload_failed", err.Error())
		return
	}
	w.Header().Set("ETag", `"`+revision+`"`)
	writeJSON(w, http.StatusOK, tailscaleResponseFrom(cfg, revision))
}

func tailscaleResponseFrom(cfg config.Config, revision string) TailscaleResponse {
	_, gatewayActive, _ := runtime.LoadState(runtime.NewPaths(cfg).StateFile)
	authKeyPresent := fileHasContent(cfg.Tailscale.AuthKeyFile)
	identityPresent := tailscaleLocalIdentityPresent(cfg.Tailscale.StateDir)
	runtimeState := "disabled"
	warnings := []string{}
	if cfg.Tailscale.Enabled {
		if gatewayActive {
			runtimeState = "available_on_demand"
		} else {
			runtimeState = "pending_gateway_start"
			warnings = append(warnings, "网关启动后生效")
		}
		if !identityPresent {
			warnings = append(warnings, "OpenSurge 会在网关启动后主动预热并注册节点；首次访问仍可能需要重试")
		}
		if !authKeyPresent && !identityPresent {
			warnings = append(warnings, "首次连接前需要设置 Auth Key")
		}
	}
	return TailscaleResponse{
		SchemaVersion: SchemaVersion,
		Revision:      revision,
		Settings: TailscaleSettings{
			Enabled:                cfg.Tailscale.Enabled,
			DisplayName:            cfg.Tailscale.DisplayName,
			Hostname:               cfg.Tailscale.Hostname,
			ControlURL:             cfg.Tailscale.ControlURL,
			AcceptRoutes:           cfg.Tailscale.AcceptRoutes,
			MagicDNSSuffixes:       append([]string{}, cfg.Tailscale.MagicDNSSuffixes...),
			PeerCIDRs:              append([]string{}, cfg.Tailscale.PeerCIDRs...),
			SubnetRoutes:           append([]string{}, cfg.Tailscale.SubnetRoutes...),
			AllowMac:               cfg.Tailscale.AllowMac,
			AllowAllDevices:        cfg.Tailscale.AllowAllDevices,
			AllowedDevices:         append([]string{}, cfg.Tailscale.AllowedDevices...),
			ExitNode:               cfg.Tailscale.ExitNode,
			ExitNodeAllowLANAccess: cfg.Tailscale.ExitNodeAllowLANAccess,
		},
		AuthKeyPresent:  authKeyPresent,
		IdentityPresent: identityPresent,
		GatewayActive:   gatewayActive,
		RuntimeState:    runtimeState,
		SelectableExit:  cfg.Tailscale.Enabled && cfg.Tailscale.ExitNode != "",
		Warnings:        warnings,
	}
}

func fileHasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
