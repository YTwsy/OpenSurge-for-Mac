package controlapi

import (
	"testing"
	"time"

	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/mihomo"
)

func TestAggregateDeviceTrafficJoinsLeasesAndActiveConnections(t *testing.T) {
	now := time.Now()
	leases := []device.Client{
		{Hostname: "iPhone-15", IP: "192.168.1.51", MAC: "AA:BB:CC:DD:EE:51", Online: true, ExpiresAt: now.Add(time.Hour)},
		{Hostname: "Apple-TV", IP: "192.168.1.88", MAC: "AA:BB:CC:DD:EE:88", Online: true, ExpiresAt: now.Add(time.Hour)},
		{IP: "192.168.1.110", MAC: "A4:5E:60:00:00:01", Online: false, ExpiresAt: now.Add(-time.Minute)},
	}
	connections := mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{
		{Upload: 100, Download: 900, Chains: []string{"流媒体组", "美国-02"}, Metadata: map[string]any{"sourceIP": "192.168.1.88"}},
		{Upload: 20, Download: 80, Chains: []string{"DIRECT"}, Metadata: map[string]any{"sourceIP": "192.168.1.51"}},
		{Upload: 10, Download: 20, Chains: []string{"备用"}, Metadata: map[string]any{"sourceIP": "192.168.1.88"}},
		{Upload: 7, Download: 11, Chains: []string{"DIRECT"}, Metadata: map[string]any{"sourceIP": "127.0.0.1"}},
		{Upload: 1, Download: 2, Metadata: map[string]any{"sourceIP": 42}},
	}}

	result := aggregateDeviceTraffic(leases, connections)
	if result.Totals.Devices != 3 || result.Totals.ActiveConnections != 3 || result.Totals.Upload != 130 || result.Totals.Download != 1000 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if result.UnmatchedConnections != 2 {
		t.Fatalf("unmatched = %d", result.UnmatchedConnections)
	}
	if len(result.Devices) != 3 || result.Devices[0].Hostname != "Apple-TV" {
		t.Fatalf("devices = %#v", result.Devices)
	}
	appleTV := result.Devices[0]
	if appleTV.ActiveConnections != 2 || appleTV.Upload != 110 || appleTV.Download != 920 || appleTV.PrimaryEgress != "流媒体组 → 美国-02" {
		t.Fatalf("Apple TV traffic = %#v", appleTV)
	}
	if result.Devices[2].IP != "192.168.1.110" || result.Devices[2].ActiveConnections != 0 || result.Devices[2].PrimaryEgress != "" {
		t.Fatalf("inactive device = %#v", result.Devices[2])
	}
}

func TestAggregateDeviceTrafficUsesNewestLeaseForMAC(t *testing.T) {
	now := time.Now()
	result := aggregateDeviceTraffic([]device.Client{
		{Hostname: "old", IP: "192.168.1.50", MAC: "AA:BB:CC:DD:EE:FF", Online: false, ExpiresAt: now.Add(-time.Hour)},
		{Hostname: "current", IP: "192.168.1.60", MAC: "aa:bb:cc:dd:ee:ff", Online: true, ExpiresAt: now.Add(time.Hour)},
	}, mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{
		{Upload: 12, Download: 34, Chains: []string{"DIRECT"}, Metadata: map[string]any{"sourceIP": "::ffff:192.168.1.60"}},
	}})
	if len(result.Devices) != 1 || result.Devices[0].Hostname != "current" || result.Devices[0].Upload != 12 {
		t.Fatalf("devices = %#v", result.Devices)
	}
}

func TestTrafficRateSamplerUsesConnectionDeltasForGatewayAndDevices(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	sampler := newTrafficRateSampler()
	leases := []device.Client{
		{Hostname: "Pixel", IP: "192.168.1.60", MAC: "aa:bb:cc:dd:ee:60", Online: true, ExpiresAt: now.Add(time.Hour)},
	}
	first := mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{
		{ID: "device", Upload: 100, Download: 1000, Metadata: map[string]any{"sourceIP": "192.168.1.60"}},
		{ID: "host", Upload: 40, Download: 400, Metadata: map[string]any{"sourceIP": "127.0.0.1"}},
	}}
	firstResponse := aggregateDeviceTraffic(leases, first)
	sampler.annotate(&firstResponse, first, now)
	if firstResponse.GatewayRates != (TrafficRates{}) || firstResponse.Devices[0].UploadRate != 0 {
		t.Fatalf("first sample rates = %#v / %#v", firstResponse.GatewayRates, firstResponse.Devices[0])
	}

	second := mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{
		{ID: "device", Upload: 2100, Download: 7000, Metadata: map[string]any{"sourceIP": "192.168.1.60"}},
		{ID: "host", Upload: 1040, Download: 2400, Metadata: map[string]any{"sourceIP": "127.0.0.1"}},
		{ID: "new", Upload: 9999, Download: 9999, Metadata: map[string]any{"sourceIP": "192.168.1.60"}},
	}}
	secondResponse := aggregateDeviceTraffic(leases, second)
	sampler.annotate(&secondResponse, second, now.Add(2*time.Second))

	if secondResponse.GatewayRates.Upload != 1500 || secondResponse.GatewayRates.Download != 4000 {
		t.Fatalf("gateway rates = %#v", secondResponse.GatewayRates)
	}
	deviceTraffic := secondResponse.Devices[0]
	if deviceTraffic.UploadRate != 1000 || deviceTraffic.DownloadRate != 3000 {
		t.Fatalf("device rates = %#v", deviceTraffic)
	}
	if secondResponse.Totals.UploadRate != 1000 || secondResponse.Totals.DownloadRate != 3000 {
		t.Fatalf("matched rates = %#v", secondResponse.Totals)
	}
}

func TestTrafficRateSamplerResetsAfterLongGap(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	sampler := newTrafficRateSampler()
	snapshot := mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{{ID: "one", Upload: 100, Download: 200}}}
	response := DeviceTrafficResponse{}
	sampler.annotate(&response, snapshot, now)

	snapshot.Connections[0].Upload = 10_000
	snapshot.Connections[0].Download = 20_000
	response = DeviceTrafficResponse{}
	sampler.annotate(&response, snapshot, now.Add(maxTrafficSampleGap+time.Second))
	if response.GatewayRates != (TrafficRates{}) {
		t.Fatalf("rates after long gap = %#v", response.GatewayRates)
	}
}

func TestRegisteredDeviceNamesOverrideMissingLeaseHostnames(t *testing.T) {
	policy := device.PolicySet{Devices: []device.ManagedDevice{
		{ID: "PlayStation-5", MAC: "90:47:48:c8:f9:1b"},
		{ID: "living-room-tv", Name: "Living Room TV", MAC: "AA:BB:CC:DD:EE:02"},
	}}
	response := DeviceTrafficResponse{Devices: []DeviceTraffic{
		{MAC: "90:47:48:c8:f9:1b"},
		{MAC: "aa:bb:cc:dd:ee:02", Hostname: "vendor-hostname"},
	}}
	annotateRegisteredDeviceNames(&response, policy)
	if response.Devices[0].Name != "PlayStation-5" || response.Devices[1].Name != "Living Room TV" {
		t.Fatalf("registered traffic names = %#v", response.Devices)
	}
	leases := []device.Client{
		{MAC: "90:47:48:C8:F9:1B"},
		{MAC: "aa:bb:cc:dd:ee:02", Hostname: "vendor-hostname"},
	}
	annotateRegisteredLeaseNames(leases, policy)
	if leases[0].RegisteredName != "PlayStation-5" || leases[1].RegisteredName != "Living Room TV" {
		t.Fatalf("registered lease names = %#v", leases)
	}
}
