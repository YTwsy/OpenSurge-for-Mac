package controlapi

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

const deviceTrafficScope = "active_sessions"

type egressUsage struct {
	connections int
	bytes       int64
}

func (s *Server) handleDeviceTraffic(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	paths := runtime.NewPaths(cfg)
	leases, err := device.LoadLeases(paths.LeaseFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "leases_unavailable", err.Error())
		return
	}
	connections, connectionErr := s.fetchConnections(r.Context(), cfg)
	response := aggregateDeviceTraffic(leases, connections)
	response.SchemaVersion = SchemaVersion
	response.Revision = fileDigest(s.configPath)
	response.SampledAt = time.Now().UTC()
	response.Scope = deviceTrafficScope
	response.ConnectionError = errorString(connectionErr)
	writeJSON(w, http.StatusOK, response)
}

func aggregateDeviceTraffic(leases []device.Client, snapshot mihomo.ConnectionsSnapshot) DeviceTrafficResponse {
	selected := selectCurrentLeases(leases)
	rows := make([]DeviceTraffic, 0, len(selected))
	byIP := make(map[string]int, len(selected))
	for _, lease := range selected {
		ip := normalizeTrafficIP(lease.IP)
		if ip == "" {
			continue
		}
		byIP[ip] = len(rows)
		rows = append(rows, DeviceTraffic{
			Hostname: lease.Hostname,
			IP:       ip,
			MAC:      strings.ToLower(strings.TrimSpace(lease.MAC)),
			Online:   lease.Online,
		})
	}

	egressByDevice := make([]map[string]egressUsage, len(rows))
	unmatched := 0
	for _, connection := range snapshot.Connections {
		sourceIP := metadataString(connection.Metadata, "sourceIP")
		index, ok := byIP[normalizeTrafficIP(sourceIP)]
		if !ok {
			unmatched++
			continue
		}
		upload := nonnegativeBytes(connection.Upload)
		download := nonnegativeBytes(connection.Download)
		rows[index].ActiveConnections++
		rows[index].Upload += upload
		rows[index].Download += download

		egress := connectionEgress(connection.Chains)
		if egress == "" {
			continue
		}
		if egressByDevice[index] == nil {
			egressByDevice[index] = map[string]egressUsage{}
		}
		usage := egressByDevice[index][egress]
		usage.connections++
		usage.bytes += upload + download
		egressByDevice[index][egress] = usage
	}

	for index := range rows {
		rows[index].PrimaryEgress = primaryEgress(egressByDevice[index])
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ActiveConnections != rows[j].ActiveConnections {
			return rows[i].ActiveConnections > rows[j].ActiveConnections
		}
		if rows[i].Online != rows[j].Online {
			return rows[i].Online
		}
		if rows[i].Hostname != rows[j].Hostname {
			return rows[i].Hostname < rows[j].Hostname
		}
		return rows[i].IP < rows[j].IP
	})

	totals := DeviceTrafficTotals{Devices: len(rows)}
	for _, row := range rows {
		totals.ActiveConnections += row.ActiveConnections
		totals.Upload += row.Upload
		totals.Download += row.Download
	}
	return DeviceTrafficResponse{Devices: rows, Totals: totals, UnmatchedConnections: unmatched}
}

func selectCurrentLeases(leases []device.Client) []device.Client {
	byIdentity := make(map[string]device.Client, len(leases))
	for _, lease := range leases {
		identity := strings.ToLower(strings.TrimSpace(lease.MAC))
		if identity == "" {
			identity = "ip:" + normalizeTrafficIP(lease.IP)
		}
		current, exists := byIdentity[identity]
		if !exists || preferTrafficLease(lease, current) {
			byIdentity[identity] = lease
		}
	}
	selected := make([]device.Client, 0, len(byIdentity))
	for _, lease := range byIdentity {
		selected = append(selected, lease)
	}
	return selected
}

func preferTrafficLease(candidate, current device.Client) bool {
	if candidate.Online != current.Online {
		return candidate.Online
	}
	return candidate.ExpiresAt.After(current.ExpiresAt)
}

func metadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func normalizeTrafficIP(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if zone := strings.LastIndexByte(value, '%'); zone >= 0 {
		value = value[:zone]
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String()
	}
	return ip.String()
}

func nonnegativeBytes(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func connectionEgress(chains []string) string {
	parts := make([]string, 0, len(chains))
	for _, chain := range chains {
		if chain = strings.TrimSpace(chain); chain != "" {
			parts = append(parts, chain)
		}
	}
	return strings.Join(parts, " → ")
}

func primaryEgress(usages map[string]egressUsage) string {
	selected := ""
	best := egressUsage{}
	for egress, usage := range usages {
		if selected == "" || usage.bytes > best.bytes ||
			(usage.bytes == best.bytes && usage.connections > best.connections) ||
			(usage.bytes == best.bytes && usage.connections == best.connections && egress < selected) {
			selected = egress
			best = usage
		}
	}
	return selected
}
