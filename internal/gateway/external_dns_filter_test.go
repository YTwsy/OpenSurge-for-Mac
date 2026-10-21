package gateway

import (
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestExternalDNSFilterRecoveryUsesAppliedConfig(t *testing.T) {
	desired := config.Default()
	desired.ExternalDNSFilter.Enabled = true
	desired.ExternalDNSFilter.Server = "192.0.2.2:5353"
	if got := appliedConfigFromState(desired, runtime.State{}); got.ExternalDNSFilter.Enabled {
		t.Fatal("legacy running state activated a new filter")
	}
	applied := config.ExternalDNSFilterConfig{Enabled: true, Server: "127.0.0.1:5353", TimeoutMS: 2000}
	got := appliedConfigFromState(desired, runtime.State{ExternalDNSFilter: &applied})
	if got.ExternalDNSFilter.Server != applied.Server || !got.ExternalDNSFilter.Enabled {
		t.Fatal(got.ExternalDNSFilter)
	}
}
