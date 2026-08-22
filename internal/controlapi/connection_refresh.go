package controlapi

import (
	"fmt"
	"net/http"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

const (
	connectionRefreshScopeLocal  = "gateway_local"
	connectionRefreshScopeDevice = "device"
)

func (s *Server) handleLocalConnectionRefresh(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	snapshot, err := s.fetchConnections(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "connections_unavailable", err.Error())
		return
	}
	localSources := gatewayLocalSourceIPs(snapshot, cfg.Gateway.LANIP)
	ids := matchingConnectionIDs(snapshot, func(connection mihomo.Connection) bool {
		return isGatewayLocalConnection(connection, cfg.Gateway.LANIP, localSources)
	})
	s.writeConnectionRefresh(w, r, cfg, connectionRefreshScopeLocal, "", ids)
}

func (s *Server) handleDeviceConnectionRefresh(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	deviceID := strings.TrimSpace(r.PathValue("device"))
	managed, found := appliedManagedDevice(runtime.NewPaths(cfg), deviceID)
	if !found {
		writeError(w, http.StatusNotFound, "applied_device_not_found", fmt.Sprintf("device %q is not part of the applied runtime policy", deviceID))
		return
	}
	if device.EffectiveGatewayTarget(managed.GatewayTarget) == device.GatewayTargetUpstreamRouter {
		writeError(w, http.StatusConflict, "device_connections_unmanaged", "this device uses the upstream router and has no connections managed by OpenSurge")
		return
	}
	sourceIP := normalizeTrafficIP(managed.IPv4)
	snapshot, err := s.fetchConnections(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "connections_unavailable", err.Error())
		return
	}
	ids := matchingConnectionIDs(snapshot, func(connection mihomo.Connection) bool {
		return normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP")) == sourceIP ||
			metadataString(connection.Metadata, "inboundUser") == mihomo.DeviceInboundUser(managed.ID)
	})
	s.writeConnectionRefresh(w, r, cfg, connectionRefreshScopeDevice, managed.ID, ids)
}

func (s *Server) writeConnectionRefresh(w http.ResponseWriter, r *http.Request, cfg config.Config, scope, deviceID string, ids []string) {
	closed, err := s.closeConnections(r.Context(), cfg, ids)
	if closed > 0 {
		s.trafficSampler.reset()
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "connection_refresh_failed", fmt.Sprintf("closed %d of %d matching connections: %v", closed, len(ids), err))
		return
	}
	writeJSON(w, http.StatusOK, ConnectionRefreshResponse{
		SchemaVersion:      SchemaVersion,
		Scope:              scope,
		DeviceID:           deviceID,
		MatchedConnections: len(ids),
		ClosedConnections:  closed,
	})
}

func appliedManagedDevice(paths runtime.Paths, id string) (device.ManagedDevice, bool) {
	policy := loadAppliedDevicePolicy(paths)
	for _, managed := range policy.Devices {
		if managed.ID == id {
			return managed, true
		}
	}
	return device.ManagedDevice{}, false
}

func matchingConnectionIDs(snapshot mihomo.ConnectionsSnapshot, matches func(mihomo.Connection) bool) []string {
	result := make([]string, 0, len(snapshot.Connections))
	seen := make(map[string]struct{}, len(snapshot.Connections))
	for _, connection := range snapshot.Connections {
		id := strings.TrimSpace(connection.ID)
		if id == "" || !matches(connection) {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
