package dhcp

import (
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestRenderConfig(t *testing.T) {
	cfg := config.Default()
	paths := runtime.NewPaths(cfg)
	rendered, err := RenderConfig(cfg, paths)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"interface=en0",
		"dhcp-range=192.168.50.100,192.168.50.200,12h",
		"dhcp-option=option:router,192.168.50.1",
		"port=53",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}
