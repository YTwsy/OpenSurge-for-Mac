package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/runtime"
)

const neighborChangeWindow = 2 * time.Minute

type neighborChange struct {
	mac string
	at  time.Time
}
type deviceNeighborHistory struct {
	mu      sync.Mutex
	scope   string
	changes map[string][]neighborChange
}

// The history is bounded, scoped to the current LAN, and ephemeral. A single
// rotation is recoverable; two transitions in the window need an ownership check.
// Serialize sampling as well as recording so concurrent GET/POST requests cannot
// turn out-of-order snapshots into an apparent MAC flap.
func (s *Server) observeSameLAN(ctx context.Context, cfg config.Config, registered []device.ManagedDevice) ([]ObservedDevice, error) {
	h := &s.neighborHistory
	h.mu.Lock()
	defer h.mu.Unlock()
	connections, connectionErr := s.fetchConnections(ctx, cfg)
	neighbors, neighborErr := s.discoverNeighbors(ctx, cfg.Gateway.Interface)
	observed := observedLANDevices(connections, neighbors, cfg.Gateway.LANIP, cfg.Gateway.LANPrefixLen, registered...)
	if err := errors.Join(connectionErr, neighborErr); err != nil {
		return observed, err
	}
	h.annotate(fmt.Sprintf("%s/%s/%d", cfg.Gateway.Interface, cfg.Gateway.LANIP, cfg.Gateway.LANPrefixLen), observed, time.Now())
	return observed, nil
}

func (h *deviceNeighborHistory) annotate(scope string, observed []ObservedDevice, now time.Time) {
	if h.scope != scope || h.changes == nil {
		h.scope, h.changes = scope, map[string][]neighborChange{}
	}
	for ip, changes := range h.changes {
		for len(changes) > 0 && now.Sub(changes[0].at) > neighborChangeWindow {
			changes = changes[1:]
		}
		if len(changes) == 0 {
			delete(h.changes, ip)
		} else {
			h.changes[ip] = changes
		}
	}
	for i := range observed {
		item := &observed[i]
		if item.MAC == "" || !item.NeighborObserved || item.ActiveConnections == 0 {
			continue
		}
		changes := h.changes[item.IP]
		if len(changes) == 0 && len(h.changes) >= 1024 {
			continue
		}
		if len(changes) == 0 || changes[len(changes)-1].mac != item.MAC {
			changes = append(changes, neighborChange{item.MAC, now})
		} else {
			changes[len(changes)-1].at = now
		}
		if len(changes) > 3 {
			changes = changes[len(changes)-3:]
		}
		h.changes[item.IP] = changes
		if len(changes) >= 3 {
			item.MACConflict = true
			macs := map[string]bool{}
			for _, change := range changes {
				macs[change.mac] = true
			}
			for mac := range macs {
				item.ObservedMACs = append(item.ObservedMACs, mac)
			}
			sort.Strings(item.ObservedMACs)
		}
	}
}

type deviceRebindRequest struct {
	DeviceID string `json:"device_id"`
	FromIPv4 string `json:"from_ipv4"`
	FromMAC  string `json:"from_mac"`
	ToIPv4   string `json:"to_ipv4"`
	ToMAC    string `json:"to_mac"`
}

func canonicalDeviceMAC(value string) string {
	mac, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		return ""
	}
	return mac.String()
}

// Rebinding changes only the two address fields of one persisted device. The
// profile, stable ID and selector namespace are never recreated. The normal
// configuration runner still validates, serializes and revision-checks the write.
func (s *Server) handleDeviceRebind(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil || cfg.Gateway.Mode != config.GatewayModeSameLAN || cfg.DevicePolicy.File == "" {
		writeError(w, http.StatusConflict, "device_rebind_unavailable", "device rebinding requires same-LAN mode")
		return
	}
	current, err := device.LoadPolicyBundle(cfg.DevicePolicy.File)
	if err != nil {
		writeError(w, http.StatusBadRequest, "device_policy_invalid", err.Error())
		return
	}
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != current.Digest {
		writeError(w, http.StatusConflict, "revision_conflict", "device configuration has changed")
		return
	}
	var request deviceRebindRequest
	if err := decodeJSON(r, &request, 4096); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	request.FromMAC, request.ToMAC = canonicalDeviceMAC(request.FromMAC), canonicalDeviceMAC(request.ToMAC)
	index := -1
	for i, item := range current.Policy.Devices {
		if item.ID == request.DeviceID {
			index = i
			break
		}
	}
	if index < 0 || request.FromMAC == "" || request.ToMAC == "" || current.Policy.Devices[index].IPv4 != request.FromIPv4 || canonicalDeviceMAC(current.Policy.Devices[index].MAC) != request.FromMAC {
		writeError(w, http.StatusConflict, "device_identity_changed", "registered device address has changed; refresh and try again")
		return
	}
	// Check both desired and running ownership; a pending removal must not let
	// another device take over an address that still belongs to the running policy.
	owners := append([]device.ManagedDevice(nil), current.Policy.Devices...)
	paths := runtime.NewPaths(cfg)
	if state, exists, stateErr := runtime.LoadState(paths.StateFile); stateErr != nil {
		writeError(w, http.StatusConflict, "device_observation_unavailable", stateErr.Error())
		return
	} else if exists && state.DevicePolicyDigest != "" {
		applied, err := device.LoadPolicyBundleSnapshot(paths.DevicePolicyApplied)
		if err != nil {
			writeError(w, http.StatusConflict, "device_observation_unavailable", err.Error())
			return
		}
		owners = append(owners, applied.Policy.Devices...)
	}
	observed, err := s.observeSameLAN(r.Context(), cfg, current.Policy.Devices)
	if err != nil {
		writeError(w, http.StatusConflict, "device_observation_unavailable", err.Error())
		return
	}
	if err := validateDeviceRebind(request, owners, observed); err != nil {
		writeError(w, http.StatusConflict, "device_identity_changed", err.Error())
		return
	}
	current.Policy.Devices[index].IPv4, current.Policy.Devices[index].MAC = request.ToIPv4, request.ToMAC
	if err := config.ValidateDevicePolicyCandidate(cfg, current.Policy); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "device_policy_validation_failed", err.Error())
		return
	}
	data, _ := json.Marshal(current.Policy)
	ctx, operation, ok := s.beginRequestOperation(w, r, "save-device-policy")
	if !ok {
		return
	}
	revision, err := s.configRunner.ApplyDevicePolicy(ctx, s.configPath, match, data)
	s.finishOperation(operation, err)
	if err != nil {
		code, status := "device_policy_write_failed", http.StatusInternalServerError
		if strings.Contains(err.Error(), "revision conflict") {
			code, status = "revision_conflict", http.StatusConflict
		}
		writeError(w, status, code, err.Error())
		return
	}
	w.Header().Set("ETag", `"`+revision+`"`)
	writeJSON(w, http.StatusOK, DevicePolicyResponse{SchemaVersion: SchemaVersion, Revision: revision, Policy: current.Policy})
}

func validateDeviceRebind(request deviceRebindRequest, owners []device.ManagedDevice, observed []ObservedDevice) error {
	ipChanged, macChanged := request.FromIPv4 != request.ToIPv4, request.FromMAC != request.ToMAC
	if ipChanged == macChanged {
		return fmt.Errorf("exactly one address may change")
	}
	for _, owner := range owners {
		if owner.ID != request.DeviceID && (owner.IPv4 == request.ToIPv4 || canonicalDeviceMAC(owner.MAC) == request.ToMAC) {
			return fmt.Errorf("address belongs to device %s", owner.ID)
		}
	}
	candidates := 0
	for _, item := range observed {
		if item.ActiveConnections == 0 {
			continue
		}
		if item.IP == request.ToIPv4 && (item.MACConflict || !item.NeighborObserved || canonicalDeviceMAC(item.MAC) != request.ToMAC) {
			return fmt.Errorf("current address evidence is ambiguous or has changed")
		}
		if ipChanged && item.IP == request.FromIPv4 {
			return fmt.Errorf("previous IP still has active traffic")
		}
		if macChanged && item.IP != request.ToIPv4 && canonicalDeviceMAC(item.MAC) == request.FromMAC {
			return fmt.Errorf("previous MAC is still active at another address")
		}
		if canonicalDeviceMAC(item.MAC) == request.ToMAC && item.NeighborObserved {
			if item.IP != request.ToIPv4 || item.MACConflict {
				return fmt.Errorf("MAC has multiple or conflicting observations")
			}
			candidates++
		}
	}
	if candidates != 1 {
		return fmt.Errorf("a unique current neighbor with active traffic is required")
	}
	return nil
}
