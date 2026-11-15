package controlapi

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/lan"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

const deviceTrafficScope = "active_sessions"

const (
	identitySourceDHCPLease        = "dhcp_lease"
	identitySourceRegisteredStatic = "registered_static"
	identitySourceObservedTraffic  = "observed_traffic"
	identitySourceGatewayLocal     = "gateway_local"
)

const (
	localTransportNone                = "none"
	localTransportTUN                 = "tun"
	localTransportExplicitProxy       = "explicit_proxy"
	localTransportTUNAndExplicitProxy = "tun_and_explicit_proxy"
	localTransportOther               = "other"
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
	s.handleConnectionObservation(w, r, false)
}

func annotateDeviceTrafficIPv6BlockState(rows []DeviceTraffic, enabled bool) {
	for index := range rows {
		rows[index].IPv6Blocked = enabled && rows[index].GatewayTarget == device.GatewayTargetUpstreamRouter
	}
}

func (s *trafficRateSampler) annotate(response *DeviceTrafficResponse, snapshot mihomo.ConnectionsSnapshot, _ gatewayLocalIdentity, sampledAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := make(map[string]trafficConnectionCounters, len(snapshot.Connections))
	for _, connection := range snapshot.Connections {
		if id := strings.TrimSpace(connection.ID); id != "" {
			current[id] = trafficConnectionCounters{upload: nonnegativeBytes(connection.Upload), download: nonnegativeBytes(connection.Download)}
		}
	}
	response.connectionRates = make([]TrafficRates, len(snapshot.Connections))
	elapsed := sampledAt.Sub(s.sampledAt)
	if !s.sampledAt.IsZero() && elapsed > 0 && elapsed <= maxTrafficSampleGap {
		byKey := map[string]*DeviceTraffic{gatewayLocalOwner: &response.GatewayLocal, unclassifiedOwner: &response.Unclassified}
		for index := range response.Devices {
			byKey[response.Devices[index].Key] = &response.Devices[index]
		}
		for index, connection := range snapshot.Connections {
			previous, ok := s.connections[strings.TrimSpace(connection.ID)]
			if !ok {
				continue
			}
			rates := TrafficRates{Upload: bytesPerSecond(counterDelta(connection.Upload, previous.upload), elapsed), Download: bytesPerSecond(counterDelta(connection.Download, previous.download), elapsed)}
			response.connectionRates[index] = rates
			response.GatewayRates.Upload += rates.Upload
			response.GatewayRates.Download += rates.Download
			if index < len(response.connectionOwners) {
				if row := byKey[response.connectionOwners[index]]; row != nil {
					row.UploadRate += rates.Upload
					row.DownloadRate += rates.Download
				}
			}
		}
	}
	for _, row := range response.Devices {
		response.Totals.UploadRate += row.UploadRate
		response.Totals.DownloadRate += row.DownloadRate
	}
	response.GatewayTotals.UploadRate = response.GatewayRates.Upload
	response.GatewayTotals.DownloadRate = response.GatewayRates.Download
	s.sampledAt, s.connections = sampledAt, current
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
		if name := byMAC[response.Devices[index].MAC]; name != "" {
			response.Devices[index].Name = name
		}
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
	return aggregateDeviceTrafficWithPolicy(leases, device.PolicySet{}, snapshot, newGatewayLocalIdentity("", mihomo.TUNRuntimeState{}), 0, false)
}

func aggregateDeviceTrafficWithPolicy(leases []device.Client, policy device.PolicySet, snapshot mihomo.ConnectionsSnapshot, localIdentity gatewayLocalIdentity, prefixLen int, observeLAN bool) DeviceTrafficResponse {
	gatewayIP := localIdentity.gatewayIP
	rows := make([]DeviceTraffic, 0, len(leases)+len(policy.Devices))
	byIP := map[string]int{}
	byID := map[string]int{}
	for _, lease := range selectCurrentLeases(leases) {
		ip := normalizeTrafficIP(lease.IP)
		if ip == "" || (gatewayIP != "" && !sameLANSourceIPv4(ip, gatewayIP, prefixLen)) {
			continue
		}
		index := len(rows)
		rows = append(rows, DeviceTraffic{Hostname: lease.Hostname, IP: ip, MAC: strings.ToLower(strings.TrimSpace(lease.MAC)), Online: lease.Online, IdentitySource: identitySourceDHCPLease, Addresses: []string{ip}})
		if _, exists := byIP[ip]; exists {
			byIP[ip] = -1
		} else {
			byIP[ip] = index
		}
	}
	for _, managed := range policy.Devices {
		ip := normalizeTrafficIP(managed.IPv4)
		if ip == "" || (gatewayIP != "" && !sameLANSourceIPv4(ip, gatewayIP, prefixLen)) {
			continue
		}
		index, exists := byIP[ip]
		if exists {
			if index < 0 || !((observeLAN && strings.TrimSpace(managed.MAC) == "") || strings.EqualFold(rows[index].MAC, managed.MAC)) {
				continue
			}
		} else {
			index = len(rows)
			byIP[ip] = index
			rows = append(rows, DeviceTraffic{IP: ip, MAC: strings.ToLower(strings.TrimSpace(managed.MAC)), IdentitySource: identitySourceRegisteredStatic, Addresses: []string{ip}})
		}
		rows[index].Name = device.DisplayName(managed)
		rows[index].DeviceID = managed.ID
		rows[index].ConfigurationState = "applied"
		rows[index].GatewayTarget = device.EffectiveGatewayTarget(managed.GatewayTarget)
		// Only MAC-backed applied identities are exported to the packet listener.
		if mac, err := net.ParseMAC(managed.MAC); err == nil && len(mac) == 6 && managed.ID != "" {
			if _, duplicate := byID[managed.ID]; duplicate {
				byID[managed.ID] = -1
			} else {
				byID[managed.ID] = index
			}
		}
	}
	for _, connection := range snapshot.Connections {
		if localIdentity.matches(connection) || hasDownstreamInboundIdentity(connection) {
			continue
		}
		ip := normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP"))
		if !observeLAN || !sameLANSourceIPv4(ip, gatewayIP, prefixLen) {
			continue
		}
		if _, exists := byIP[ip]; exists {
			continue
		}
		byIP[ip] = len(rows)
		rows = append(rows, DeviceTraffic{IP: ip, Online: true, IdentitySource: identitySourceObservedTraffic, Addresses: []string{ip}})
	}
	for index := range rows {
		rows[index].Key = trafficDeviceKey(rows[index])
	}
	local := DeviceTraffic{Key: gatewayLocalOwner, IP: normalizeTrafficIP(gatewayIP), IdentitySource: identitySourceGatewayLocal, Transport: localTransportNone, Addresses: []string{}}
	unknown := DeviceTraffic{Key: unclassifiedOwner, IdentitySource: unclassifiedOwner, Addresses: []string{}}
	egress := map[string]map[string]egressUsage{}
	owners := make([]string, len(snapshot.Connections))
	localHasTUN, localHasExplicit := false, false
	gatewayTotals := DeviceTrafficTotals{ActiveConnections: len(snapshot.Connections)}
	for index, connection := range snapshot.Connections {
		row := &unknown
		if localIdentity.matches(connection) {
			row = &local
			switch localConnectionTransport(connection) {
			case localTransportTUN:
				localHasTUN = true
			case localTransportExplicitProxy:
				localHasExplicit = true
			}
		} else if hasDownstreamInboundIdentity(connection) {
			if deviceIndex, ok := byID[packetTrafficDevice(connection)]; ok && deviceIndex >= 0 {
				row = &rows[deviceIndex]
			}
		} else if deviceIndex, ok := byIP[normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP"))]; ok && deviceIndex >= 0 {
			row = &rows[deviceIndex]
		}
		owners[index] = row.Key
		row.ActiveConnections++
		row.Online = true
		row.Upload += nonnegativeBytes(connection.Upload)
		row.Download += nonnegativeBytes(connection.Download)
		addTrafficAddress(row, normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP")))
		gatewayTotals.Upload += nonnegativeBytes(connection.Upload)
		gatewayTotals.Download += nonnegativeBytes(connection.Download)
		if chain := connectionEgress(connection.Chains); chain != "" {
			if egress[row.Key] == nil {
				egress[row.Key] = map[string]egressUsage{}
			}
			usage := egress[row.Key][chain]
			usage.connections++
			usage.bytes += nonnegativeBytes(connection.Upload) + nonnegativeBytes(connection.Download)
			egress[row.Key][chain] = usage
		}
	}
	for index := range rows {
		rows[index].PrimaryEgress = primaryEgress(egress[rows[index].Key])
		sort.Strings(rows[index].Addresses)
		if rows[index].GatewayTarget == device.GatewayTargetUpstreamRouter && rows[index].PrimaryEgress == "" {
			rows[index].PrimaryEgress = "主路由直连"
		}
	}
	local.PrimaryEgress = primaryEgress(egress[gatewayLocalOwner])
	local.Transport = combinedLocalTransport(localHasTUN, localHasExplicit, local.ActiveConnections > 0)
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
		return rows[i].Key < rows[j].Key
	})
	totals := DeviceTrafficTotals{Devices: len(rows)}
	unidentified := 0
	for _, row := range rows {
		totals.ActiveConnections += row.ActiveConnections
		totals.Upload += row.Upload
		totals.Download += row.Download
		if row.IdentitySource == identitySourceObservedTraffic {
			unidentified += row.ActiveConnections
		}
	}
	return DeviceTrafficResponse{
		GatewayLocal: local, Devices: rows, Totals: totals, GatewayTotals: gatewayTotals, Unclassified: unknown,
		UnidentifiedDeviceConnections: unidentified, UnclassifiedConnections: unknown.ActiveConnections,
		UnmatchedConnections: local.ActiveConnections + unknown.ActiveConnections, connectionOwners: owners,
	}
}

func isLoopbackIP(value string) bool {
	ip := net.ParseIP(value)
	return ip != nil && ip.IsLoopback()
}

func localConnectionTransport(connection mihomo.Connection) string {
	switch strings.ToLower(metadataString(connection.Metadata, "type")) {
	case "tun":
		return localTransportTUN
	case "http", "https", "mixed", "socks", "socks4", "socks5":
		return localTransportExplicitProxy
	default:
		return localTransportOther
	}
}

func combinedLocalTransport(hasTUN, hasExplicitProxy, hasConnections bool) string {
	switch {
	case hasTUN && hasExplicitProxy:
		return localTransportTUNAndExplicitProxy
	case hasTUN:
		return localTransportTUN
	case hasExplicitProxy:
		return localTransportExplicitProxy
	case hasConnections:
		return localTransportOther
	default:
		return localTransportNone
	}
}

func loadAppliedDevicePolicy(paths runtime.Paths) device.PolicySet {
	bundle, _ := loadAppliedDevicePolicyBundle(paths)
	return bundle.Policy
}

func loadAppliedDevicePolicyBundle(paths runtime.Paths) (device.PolicyBundle, bool) {
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil || !exists || state.DevicePolicyDigest == "" {
		return device.PolicyBundle{}, false
	}
	bundle, err := device.LoadPolicyBundleSnapshot(paths.DevicePolicyApplied)
	if err != nil || bundle.Digest != state.DevicePolicyDigest {
		return device.PolicyBundle{}, false
	}
	return bundle, true
}

func observedLANDevices(snapshot mihomo.ConnectionsSnapshot, neighbors []macosnetwork.Neighbor, gatewayIP string, prefixLen int, registered ...device.ManagedDevice) []ObservedDevice {
	neighborByIP := make(map[string]string, len(neighbors))
	ambiguousNeighborIP := make(map[string]bool)
	macsByIP := make(map[string]map[string]bool)
	for _, neighbor := range neighbors {
		if ip := normalizeTrafficIP(neighbor.IP); ip != "" {
			mac := canonicalDeviceMAC(neighbor.MAC)
			if mac != "" {
				if macsByIP[ip] == nil {
					macsByIP[ip] = map[string]bool{}
				}
				macsByIP[ip][mac] = true
			}
			if mac == "" || ambiguousNeighborIP[ip] {
				continue
			}
			if current := neighborByIP[ip]; current != "" && current != mac {
				delete(neighborByIP, ip)
				ambiguousNeighborIP[ip] = true
				continue
			}
			neighborByIP[ip] = mac
		}
	}
	byIP := map[string]*ObservedDevice{}
	for _, connection := range snapshot.Connections {
		ip := normalizeTrafficIP(metadataString(connection.Metadata, "sourceIP"))
		if !sameLANSourceIPv4(ip, gatewayIP, prefixLen) {
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
	// A topology migration is configured while the gateway is stopped, so
	// mihomo may have no active connection to seed the observation list. Expose
	// neighbor-only evidence solely for already registered IP-only devices;
	// never turn the whole ARP cache into registration candidates.
	for _, managed := range registered {
		if strings.TrimSpace(managed.MAC) != "" {
			continue
		}
		ip := normalizeTrafficIP(managed.IPv4)
		mac := neighborByIP[ip]
		if mac == "" || !sameLANSourceIPv4(ip, gatewayIP, prefixLen) {
			continue
		}
		if observed := byIP[ip]; observed == nil {
			byIP[ip] = &ObservedDevice{IP: ip, MAC: mac, NeighborObserved: true}
		}
	}
	result := make([]ObservedDevice, 0, len(byIP))
	for _, observed := range byIP {
		if ambiguousNeighborIP[observed.IP] {
			observed.MACConflict = true
			for mac := range macsByIP[observed.IP] {
				observed.ObservedMACs = append(observed.ObservedMACs, mac)
			}
			sort.Strings(observed.ObservedMACs)
		}
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

func sameLANSourceIPv4(value, gatewayIP string, prefixLen int) bool {
	ip := net.ParseIP(strings.TrimSpace(value)).To4()
	scope, err := lan.NewScope(strings.TrimSpace(gatewayIP), prefixLen)
	if ip == nil || err != nil || ip.Equal(scope.Gateway) {
		return false
	}
	return scope.UsableHost(ip)
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
