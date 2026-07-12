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
