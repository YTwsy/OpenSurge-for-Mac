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
		"pass in all",
		"pass out all",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered anchor missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "rdr pass") {
		t.Fatalf("rendered anchor enables TCP redirection by default:\n%s", rendered)
	}
}

func TestRenderAnchorNeverEmitsTCPRedirect(t *testing.T) {
	cfg := config.Default()
	cfg.PF.RedirectTCPTo = 7892
	rendered, err := RenderAnchor(cfg)
	if err != nil {
		t.Fatalf("RenderAnchor() error = %v", err)
	}
	if strings.Contains(rendered, "rdr pass") {
		t.Fatalf("rendered anchor emits unsupported TCP redirection:\n%s", rendered)
	}
}

func TestRenderAnchorSameLANExcludesLocalLAN(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameLAN
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.Gateway.LANIP = "192.168.1.20"
	rendered, err := RenderAnchor(cfg)
	if err != nil {
		t.Fatalf("RenderAnchor() error = %v", err)
	}

	want := "nat on en0 from 192.168.1.0/24 to ! 192.168.1.0/24 -> (en0)"
	if !strings.Contains(rendered, want) {
		t.Fatalf("rendered same-LAN anchor missing %q:\n%s", want, rendered)
	}
	if strings.Contains(rendered, "to any") {
		t.Fatalf("rendered same-LAN anchor did not exclude local LAN:\n%s", rendered)
	}
}

func TestRenderAnchorSameWiFiDHCPExcludesLocalLAN(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameWiFiDHCP
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.Gateway.LANIP = "192.168.1.20"

	rendered, err := RenderAnchor(cfg)
	if err != nil {
		t.Fatalf("RenderAnchor() error = %v", err)
	}
	want := "nat on en0 from 192.168.1.0/24 to ! 192.168.1.0/24 -> (en0)"
	if !strings.Contains(rendered, want) {
		t.Fatalf("rendered same-WiFi DHCP anchor missing %q:\n%s", want, rendered)
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

func TestSplitAnchor(t *testing.T) {
	parent, child, nested := splitAnchor("com.apple/open_mihomo_gateway")
	if parent != "com.apple" || child != "open_mihomo_gateway" || !nested {
		t.Fatalf("splitAnchor nested = (%q, %q, %v)", parent, child, nested)
	}

	parent, child, nested = splitAnchor("open_mihomo_gateway")
	if parent != "" || child != "open_mihomo_gateway" || nested {
		t.Fatalf("splitAnchor root = (%q, %q, %v)", parent, child, nested)
	}
}

func TestAnchorOutputContains(t *testing.T) {
	output := "com.apple\nopen_mihomo_gateway\n"
	if !anchorOutputContains(output, "open_mihomo_gateway") {
		t.Fatalf("anchorOutputContains did not find nested child")
	}
	if anchorOutputContains(output, "missing") {
		t.Fatalf("anchorOutputContains found unexpected anchor")
	}
}
