package controlapi

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

const deviceTrafficScope = "active_sessions"

const (
	identitySourceDHCPLease        = "dhcp_lease"
	identitySourceRegisteredStatic = "registered_static"
	identitySourceObservedTraffic  = "observed_traffic"
)

const maxTrafficSampleGap = 15 * time.Second

type egressUsage struct {
	connections int
	bytes       int64
}

type trafficConnectionCounters struct {
	upload   int64
	download int64
}

type trafficRateSampler struct {
	mu          sync.Mutex
	sampledAt   time.Time
	connections map[string]trafficConnectionCounters
}

func newTrafficRateSampler() *trafficRateSampler {
	return &trafficRateSampler{connections: map[string]trafficConnectionCounters{}}
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
	sampledAt := time.Now().UTC()
	appliedPolicy := device.PolicySet{}
	gatewayIP := ""
	if cfg.Gateway.Mode == config.GatewayModeSameLAN {
		appliedPolicy = loadAppliedDevicePolicy(paths)
		gatewayIP = cfg.Gateway.LANIP
	}
	response := aggregateDeviceTrafficWithPolicy(leases, appliedPolicy, connections, gatewayIP)
	if connectionErr == nil {
		s.trafficSampler.annotate(&response, connections, sampledAt)
	} else {
		s.trafficSampler.reset()
	}
	if cfg.DevicePolicy.File != "" {
		if bundle, policyErr := device.LoadPolicyBundle(cfg.DevicePolicy.File); policyErr == nil {
			annotateRegisteredDeviceNames(&response, bundle.Policy)
		}
	}
	response.SchemaVersion = SchemaVersion
	response.Revision = fileDigest(s.configPath)
	response.SampledAt = sampledAt
	response.Scope = deviceTrafficScope
	response.ConnectionError = errorString(connectionErr)
	writeJSON(w, http.StatusOK, response)
}

func (s *trafficRateSampler) annotate(response *DeviceTrafficResponse, snapshot mihomo.ConnectionsSnapshot, sampledAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := make(map[string]trafficConnectionCounters, len(snapshot.Connections))
	for _, connection := range snapshot.Connections {
		if id := strings.TrimSpace(connection.ID); id != "" {
			current[id] = trafficConnectionCounters{
				upload:   nonnegativeBytes(connection.Upload),
				download: nonnegativeBytes(connection.Download),
			}
		}
	}

	elapsed := sampledAt.Sub(s.sampledAt)
	if s.sampledAt.IsZero() || elapsed <= 0 || elapsed > maxTrafficSampleGap {
		s.sampledAt = sampledAt
		s.connections = current
		return
	}

	byIP := make(map[string]*DeviceTraffic, len(response.Devices))
	for index := range response.Devices {
		byIP[response.Devices[index].IP] = &response.Devices[index]
	}

	for _, connection := range snapshot.Connections {
		id := strings.TrimSpace(connection.ID)
		previous, ok := s.connections[id]
		if id == "" || !ok {
			continue
		}
		uploadDelta := counterDelta(connection.Upload, previous.upload)
		downloadDelta := counterDelta(connection.Download, previous.download)
		response.GatewayRates.Upload += bytesPerSecond(uploadDelta, elapsed)
		response.GatewayRates.Download += bytesPerSecond(downloadDelta, elapsed)

		sourceIP := normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP"))
		if row := byIP[sourceIP]; row != nil {
			row.UploadRate += bytesPerSecond(uploadDelta, elapsed)
			row.DownloadRate += bytesPerSecond(downloadDelta, elapsed)
		}
	}

	for _, row := range response.Devices {
		response.Totals.UploadRate += row.UploadRate
		response.Totals.DownloadRate += row.DownloadRate
	}
	s.sampledAt = sampledAt
	s.connections = current
}

func (s *trafficRateSampler) reset() {
	s.mu.Lock()
	s.sampledAt = time.Time{}
	s.connections = map[string]trafficConnectionCounters{}
	s.mu.Unlock()
}

func counterDelta(current, previous int64) int64 {
	current = nonnegativeBytes(current)
	previous = nonnegativeBytes(previous)
	if current <= previous {
		return 0
	}
	return current - previous
}

func bytesPerSecond(delta int64, elapsed time.Duration) int64 {
	if delta <= 0 || elapsed <= 0 {
		return 0
	}
	return int64(float64(delta) / elapsed.Seconds())
}

func annotateRegisteredDeviceNames(response *DeviceTrafficResponse, policy device.PolicySet) {
	byMAC := registeredDeviceNames(policy)
	for index := range response.Devices {
		response.Devices[index].Name = byMAC[response.Devices[index].MAC]
	}
}

func annotateRegisteredLeaseNames(leases []device.Client, policy device.PolicySet) {
	byMAC := registeredDeviceNames(policy)
	for index := range leases {
		leases[index].RegisteredName = byMAC[strings.ToLower(strings.TrimSpace(leases[index].MAC))]
	}
}

func registeredDeviceNames(policy device.PolicySet) map[string]string {
	byMAC := make(map[string]string, len(policy.Devices))
	for _, managed := range policy.Devices {
		mac := strings.ToLower(strings.TrimSpace(managed.MAC))
		if mac != "" {
			byMAC[mac] = device.DisplayName(managed)
		}
	}
	return byMAC
}

func aggregateDeviceTraffic(leases []device.Client, snapshot mihomo.ConnectionsSnapshot) DeviceTrafficResponse {
	return aggregateDeviceTrafficWithPolicy(leases, device.PolicySet{}, snapshot, "")
}

func aggregateDeviceTrafficWithPolicy(leases []device.Client, policy device.PolicySet, snapshot mihomo.ConnectionsSnapshot, gatewayIP string) DeviceTrafficResponse {
	selected := selectCurrentLeases(leases)
	rows := make([]DeviceTraffic, 0, len(selected)+len(policy.Devices))
	byIP := make(map[string]int, len(selected)+len(policy.Devices))
	for _, lease := range selected {
		ip := normalizeTrafficIP(lease.IP)
		if ip == "" {
			continue
		}
		byIP[ip] = len(rows)
		rows = append(rows, DeviceTraffic{
			Hostname:       lease.Hostname,
			IP:             ip,
			MAC:            strings.ToLower(strings.TrimSpace(lease.MAC)),
			Online:         lease.Online,
			IdentitySource: identitySourceDHCPLease,
		})
	}
	for _, managed := range policy.Devices {
		ip := normalizeTrafficIP(managed.IPv4)
		if ip == "" || (gatewayIP != "" && !sameLANSourceIPv4(ip, gatewayIP)) {
			continue
		}
		if index, exists := byIP[ip]; exists {
			if strings.EqualFold(rows[index].MAC, managed.MAC) {
				rows[index].Name = device.DisplayName(managed)
			}
			continue
		}
		byIP[ip] = len(rows)
		rows = append(rows, DeviceTraffic{
			Name:           device.DisplayName(managed),
			IP:             ip,
			MAC:            strings.ToLower(strings.TrimSpace(managed.MAC)),
			IdentitySource: identitySourceRegisteredStatic,
		})
	}
	for _, connection := range snapshot.Connections {
		sourceIP := normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP"))
		if !sameLANSourceIPv4(sourceIP, gatewayIP) {
			continue
		}
		if _, exists := byIP[sourceIP]; exists {
			continue
		}
		byIP[sourceIP] = len(rows)
		rows = append(rows, DeviceTraffic{
			IP:             sourceIP,
			Online:         true,
			IdentitySource: identitySourceObservedTraffic,
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
		rows[index].Online = true
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

func loadAppliedDevicePolicy(paths runtime.Paths) device.PolicySet {
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil || !exists || state.DevicePolicyDigest == "" {
		return device.PolicySet{}
	}
	bundle, err := device.LoadPolicyBundleSnapshot(paths.DevicePolicyApplied)
	if err != nil || bundle.Digest != state.DevicePolicyDigest {
		return device.PolicySet{}
	}
	return bundle.Policy
}

func observedLANDevices(snapshot mihomo.ConnectionsSnapshot, neighbors []macosnetwork.Neighbor, gatewayIP string) []ObservedDevice {
	neighborByIP := make(map[string]string, len(neighbors))
	for _, neighbor := range neighbors {
		if ip := normalizeTrafficIP(neighbor.IP); ip != "" {
			neighborByIP[ip] = strings.ToLower(strings.TrimSpace(neighbor.MAC))
		}
	}
	byIP := map[string]*ObservedDevice{}
	for _, connection := range snapshot.Connections {
		ip := normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP"))
		if !sameLANSourceIPv4(ip, gatewayIP) {
			continue
		}
		observed := byIP[ip]
		if observed == nil {
			mac := neighborByIP[ip]
			observed = &ObservedDevice{IP: ip, MAC: mac, NeighborObserved: mac != ""}
			byIP[ip] = observed
		}
		observed.ActiveConnections++
	}
	result := make([]ObservedDevice, 0, len(byIP))
	for _, observed := range byIP {
		result = append(result, *observed)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ActiveConnections != result[j].ActiveConnections {
			return result[i].ActiveConnections > result[j].ActiveConnections
		}
		return result[i].IP < result[j].IP
	})
	return result
}

func sameLANSourceIPv4(value, gatewayIP string) bool {
	ip := net.ParseIP(strings.TrimSpace(value)).To4()
	gateway := net.ParseIP(strings.TrimSpace(gatewayIP)).To4()
	if ip == nil || gateway == nil || ip.Equal(gateway) {
		return false
	}
	return ip[0] == gateway[0] && ip[1] == gateway[1] && ip[2] == gateway[2] && ip[3] != 0 && ip[3] != 255
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
