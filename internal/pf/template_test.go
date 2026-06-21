package pf

import (
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestRenderAnchor(t *testing.T) {
	cfg := config.Default()
	rendered, err := RenderAnchor(cfg)
	if err != nil {
		t.Fatalf("RenderAnchor() error = %v", err)
	}

	for _, want := range []string{
		"nat on en0 from 192.168.50.0/24 to any -> (en0)",
		"rdr pass on en0 proto tcp from 192.168.50.0/24 to any -> 127.0.0.1 port 7892",
		"pass in all",
		"pass out all",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered anchor missing %q:\n%s", want, rendered)
		}
	}
}

func TestParseEnabled(t *testing.T) {
	if !parseEnabled("Status: Enabled for 0 days\n") {
		t.Fatalf("enabled status parsed as false")
	}
	if parseEnabled("Status: Disabled\n") {
		t.Fatalf("disabled status parsed as true")
	}
}
