package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/device"
)

func TestValidateRejectsMihomoRedirPort(t *testing.T) {
	cfg := Default()
	cfg.Mihomo.RedirPort = 7892

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("Validate() succeeded with unsupported mihomo.redir_port")
	}
	if !strings.Contains(err.Error(), `use transparent.mode: "tun"`) {
		t.Fatalf("Validate() error = %q", err)
	}
}

func TestValidateRejectsPFRedirectTCPTo(t *testing.T) {
	cfg := Default()
	cfg.PF.RedirectTCPTo = 7892

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("Validate() succeeded with unsupported pf.redirect_tcp_to")
	}
	if !strings.Contains(err.Error(), `use transparent.mode: "tun"`) {
		t.Fatalf("Validate() error = %q", err)
	}
}

func TestValidateAcceptsTUNTransparentMode(t *testing.T) {
	cfg := Default()
	cfg.Transparent.Mode = TransparentModeTUN

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateLocalSystemProxyRequiresTUN(t *testing.T) {
	cfg := Default()
	cfg.LocalSystemProxy.Enabled = true

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), `requires transparent.mode: "tun"`) {
		t.Fatalf("Validate() error = %v", err)
	}

	cfg.Transparent.Mode = TransparentModeTUN
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() with TUN error = %v", err)
	}
}

func TestValidateAcceptsSameLANGatewayMode(t *testing.T) {
	cfg := Default()
	cfg.Gateway.Mode = GatewayModeSameLAN
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.DHCP.Enabled = false
	cfg.Transparent.Mode = TransparentModeTUN

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPrepareDevicePolicyPausesIPOnlyDevicesOutsideSameLAN(t *testing.T) {
	policy := device.PolicySet{
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices:  []device.ManagedDevice{{ID: "speaker", IPv4: "192.168.50.101", Profile: "home"}},
	}
	bundle, err := device.CompilePolicyBundle(policy)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
	cfg.DevicePolicy.File = "already-loaded.json"
	cfg.DevicePolicy.Bundle = &bundle
	if err := PrepareDevicePolicy(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DevicePolicy.Bundle.IPOnlyDevicesActive || len(cfg.DevicePolicy.Bundle.Compiled.Devices) != 0 || len(cfg.DevicePolicy.Bundle.Policy.Devices) != 1 {
		t.Fatalf("DHCP bundle = %#v", cfg.DevicePolicy.Bundle)
	}

	cfg.Gateway.Mode = GatewayModeSameLAN
	if err := PrepareDevicePolicy(&cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.DevicePolicy.Bundle.IPOnlyDevicesActive || len(cfg.DevicePolicy.Bundle.Compiled.Devices) != 1 {
		t.Fatalf("same-LAN bundle = %#v", cfg.DevicePolicy.Bundle)
	}
}

func TestPrepareDevicePolicyRecompilesRuntimeForConfiguredLAN(t *testing.T) {
	policy := device.PolicySet{
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices: []device.ManagedDevice{
			{ID: "active", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.50.101", Profile: "home", EgressMode: device.EgressModeDedicated},
			{ID: "wider-lan", MAC: "aa:bb:cc:dd:ee:02", IPv4: "192.168.51.102", Profile: "home", EgressMode: device.EgressModeDedicated},
			{ID: "dormant", MAC: "aa:bb:cc:dd:ee:03", IPv4: "192.168.60.103", Profile: "home", EgressMode: device.EgressModeDedicated},
		},
	}
	bundle, err := device.CompilePolicyBundle(policy)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
	cfg.DevicePolicy.File = "already-loaded.json"
	cfg.DevicePolicy.Bundle = &bundle
	if err := PrepareDevicePolicy(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DevicePolicy.Bundle.ActiveLAN != "192.168.50.0/24" || len(cfg.DevicePolicy.Bundle.Policy.Devices) != 3 || len(cfg.DevicePolicy.Bundle.Compiled.Devices) != 1 || cfg.DevicePolicy.Bundle.Compiled.Devices[0].ID != "active" {
		t.Fatalf("/24 bundle = %#v", cfg.DevicePolicy.Bundle)
	}

	cfg.Gateway.LANPrefixLen = 22
	if err := PrepareDevicePolicy(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DevicePolicy.Bundle.ActiveLAN != "192.168.48.0/22" || len(cfg.DevicePolicy.Bundle.Policy.Devices) != 3 || len(cfg.DevicePolicy.Bundle.Compiled.Devices) != 2 || cfg.DevicePolicy.Bundle.Compiled.Devices[1].ID != "wider-lan" {
		t.Fatalf("/22 bundle = %#v", cfg.DevicePolicy.Bundle)
	}
}

func TestValidateDNSUpstream(t *testing.T) {
	for _, value := range []string{"", MihomoDNSUpstream, "1.1.1.1", "8.8.8.8#5353"} {
		cfg := Default()
		cfg.DNS.Upstream = value
		if err := Validate(cfg); err != nil {
			t.Fatalf("Validate() rejected dns.upstream %q: %v", value, err)
		}
	}
	for _, value := range []string{"dns.example", "1.1.1.1#0", "1.1.1.1#70000", "1.1.1.1#53#54", "1.1.1.1\nserver=8.8.8.8"} {
		cfg := Default()
		cfg.DNS.Upstream = value
		if err := Validate(cfg); err == nil {
			t.Fatalf("Validate() accepted dns.upstream %q", value)
		}
	}
}

func TestValidateRejectsInvalidSameLANConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "dhcp enabled",
			edit: func(cfg *Config) {
				cfg.DHCP.Enabled = true
			},
			want: "dhcp.enabled: false",
		},
		{
			name: "transparent off",
			edit: func(cfg *Config) {
				cfg.Transparent.Mode = TransparentModeOff
			},
			want: `transparent.mode: "tun"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Gateway.Mode = GatewayModeSameLAN
			cfg.Gateway.Interface = "en0"
			cfg.Gateway.UpstreamInterface = "en0"
			cfg.DHCP.Enabled = false
			cfg.Transparent.Mode = TransparentModeTUN
			tt.edit(&cfg)

			err := Validate(cfg)
			if err == nil {
				t.Fatalf("Validate() succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateSameWiFiDHCPGatewayMode(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "accepts a protected range outside the gateway address",
			edit: func(cfg *Config) {},
		},
		{
			name: "requires DHCP",
			edit: func(cfg *Config) { cfg.DHCP.Enabled = false },
			want: "dhcp.enabled: true",
		},
		{
			name: "requires TUN",
			edit: func(cfg *Config) { cfg.Transparent.Mode = TransparentModeOff },
			want: `transparent.mode: "tun"`,
		},
		{
			name: "rejects a range outside the LAN subnet",
			edit: func(cfg *Config) { cfg.DHCP.RangeStart = "192.168.2.120" },
			want: "DHCP range to remain",
		},
		{
			name: "rejects a range end outside the LAN subnet",
			edit: func(cfg *Config) { cfg.DHCP.RangeEnd = "192.168.2.199" },
			want: "DHCP range to remain",
		},
		{
			name: "rejects a reversed range",
			edit: func(cfg *Config) {
				cfg.DHCP.RangeStart = "192.168.1.199"
				cfg.DHCP.RangeEnd = "192.168.1.120"
			},
			want: "dhcp.range_start must not be after",
		},
		{
			name: "rejects the broadcast address in the range",
			edit: func(cfg *Config) { cfg.DHCP.RangeEnd = "192.168.1.255" },
			want: "network or broadcast",
		},
		{
			name: "rejects gateway address in the range",
			edit: func(cfg *Config) {
				cfg.DHCP.RangeStart = "192.168.1.20"
				cfg.DHCP.RangeEnd = "192.168.1.199"
			},
			want: "gateway.lan_ip must not be inside",
		},
		{
			name: "accepts a range that a wider configured prefix makes local",
			edit: func(cfg *Config) {
				cfg.Gateway.LANPrefixLen = 22
				cfg.DHCP.RangeStart = "192.168.2.120"
				cfg.DHCP.RangeEnd = "192.168.2.199"
			},
			want: "",
		},
		{
			name: "rejects a prefix length with no usable hosts",
			edit: func(cfg *Config) { cfg.Gateway.LANPrefixLen = 31 },
			want: "gateway.lan_prefix_len",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
			cfg.Gateway.Interface = "en0"
			cfg.Gateway.UpstreamInterface = "en0"
			cfg.Gateway.LANIP = "192.168.1.20"
			cfg.DHCP.Enabled = true
			cfg.DHCP.RangeStart = "192.168.1.120"
			cfg.DHCP.RangeEnd = "192.168.1.199"
			cfg.Transparent.Mode = TransparentModeTUN
			tt.edit(&cfg)

			err := Validate(cfg)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateDevicePolicyCandidateEnforcesRouterBypassTopology(t *testing.T) {
	baseConfig := func() Config {
		cfg := Default()
		cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
		cfg.Gateway.Interface = "en0"
		cfg.Gateway.UpstreamInterface = "en0"
		cfg.Gateway.LANIP = "192.168.1.20"
		cfg.DHCP.Enabled = true
		cfg.DHCP.RangeStart = "192.168.1.120"
		cfg.DHCP.RangeEnd = "192.168.1.199"
		cfg.DHCP.BypassGateway = "192.168.1.1"
		cfg.DHCP.BypassDNS = []string{"192.168.1.1", "1.1.1.1"}
		cfg.DNS.Listen = cfg.Gateway.LANIP
		cfg.Transparent.Mode = TransparentModeTUN
		return cfg
	}
	policy := device.PolicySet{
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices: []device.ManagedDevice{{
			ID: "console", MAC: "aa:bb:cc:dd:ee:05", IPv4: "192.168.1.190", Profile: "home", GatewayTarget: device.GatewayTargetUpstreamRouter,
		}},
	}

	if err := ValidateDevicePolicyCandidate(baseConfig(), policy); err != nil {
		t.Fatalf("valid router bypass rejected: %v", err)
	}
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{name: "missing gateway", edit: func(cfg *Config) { cfg.DHCP.BypassGateway = "" }, want: "requires dhcp.bypass_gateway"},
		{name: "missing DNS", edit: func(cfg *Config) { cfg.DHCP.BypassDNS = nil }, want: "requires at least one dhcp.bypass_dns"},
		{name: "different subnet", edit: func(cfg *Config) { cfg.DHCP.BypassGateway = "192.168.2.1" }, want: "must remain in gateway LAN"},
		{name: "inside pool", edit: func(cfg *Config) { cfg.DHCP.BypassGateway = "192.168.1.150" }, want: "must not be inside the DHCP range"},
		{name: "unsupported topology", edit: func(cfg *Config) { cfg.Gateway.Mode = GatewayModeIsolatedLAN }, want: "requires gateway.mode same_wifi_dhcp or same_lan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.edit(&cfg)
			err := ValidateDevicePolicyCandidate(cfg, policy)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateDevicePolicyCandidate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRouterBypassPolicySurvivesSameLANModeSwitch(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	policyJSON := []byte(`{
  "profiles": [{"id":"home","default_policies":["DIRECT"]}],
  "devices": [
    {"id":"console","mac":"aa:bb:cc:dd:ee:05","ipv4":"192.168.50.190","profile":"home","gateway_target":"upstream_router","egress_mode":"dedicated"},
    {"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.191","profile":"home","egress_mode":"dedicated"}
  ]
}`)
	if err := os.WriteFile(policyPath, policyJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.DevicePolicy.File = policyPath
	cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
	cfg.Transparent.Mode = TransparentModeTUN
	cfg.DHCP.BypassGateway = "192.168.50.254"
	cfg.DHCP.BypassDNS = []string{"192.168.50.254"}
	if err := PrepareDevicePolicy(&cfg); err != nil {
		t.Fatal(err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("DHCP takeover: %v", err)
	}
	digest := cfg.DevicePolicy.Bundle.Digest

	// The same persisted document must remain usable when DHCP is disabled,
	// including when the old DHCP-only gateway settings no longer apply.
	cfg.Gateway.Mode = GatewayModeSameLAN
	cfg.DHCP.Enabled = false
	for _, bypassGateway := range []string{"", "192.168.60.1", "192.168.50.150", "192.168.50.191"} {
		t.Run("retained gateway "+bypassGateway, func(t *testing.T) {
			candidate := cfg
			candidate.DHCP.BypassGateway = bypassGateway
			candidate.DHCP.BypassDNS = nil
			if err := ValidateDevicePolicyCandidate(candidate, cfg.DevicePolicy.Bundle.Policy); err != nil {
				t.Fatalf("same-LAN policy save: %v", err)
			}
			configPath := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(Render(candidate)), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(configPath)
			if err != nil {
				t.Fatalf("same-LAN startup config: %v", err)
			}
			bundle := loaded.DevicePolicy.Bundle
			if bundle.Digest != digest || len(bundle.Compiled.Devices) != 2 || bundle.Compiled.Devices[0].GatewayTarget != device.GatewayTargetUpstreamRouter {
				t.Fatalf("retained policy = %#v", bundle)
			}
			if len(bundle.Compiled.SelectorGroups) != 1 || bundle.Compiled.SelectorGroups[0].Name != "device/phone/default" {
				t.Fatalf("unexpected device routes: %#v", bundle.Compiled.SelectorGroups)
			}
			protected := loaded
			protected.DevicePolicy.ProtectedIPv4 = []string{"192.168.50.191"}
			if err := Validate(protected); err == nil || !strings.Contains(err.Error(), "protected IPv4") {
				t.Fatalf("explicit protected address was not enforced: %v", err)
			}
			loaded.Gateway.Mode = GatewayModeSameWiFiDHCP
			loaded.DHCP.Enabled = true
			if err := Validate(loaded); err == nil {
				t.Fatal("returning to DHCP accepted invalid bypass settings")
			}
			loaded.DHCP.BypassGateway = "192.168.50.254"
			loaded.DHCP.BypassDNS = []string{"192.168.50.254"}
			if err := Validate(loaded); err != nil {
				t.Fatalf("returning to DHCP with valid settings: %v", err)
			}
		})
	}
	retained, err := os.ReadFile(policyPath)
	if err != nil || string(retained) != string(policyJSON) {
		t.Fatalf("mode switch changed the saved device policy: %s, %v", retained, err)
	}
}

func TestValidateDevicePolicyCandidateKeepsRegistrationsFromAnotherLAN(t *testing.T) {
	cfg := Default()
	cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.Gateway.LANIP = "192.168.1.20"
	cfg.DHCP.Enabled = true
	cfg.DHCP.RangeStart = "192.168.1.120"
	cfg.DHCP.RangeEnd = "192.168.1.199"
	cfg.DNS.Listen = cfg.Gateway.LANIP
	cfg.Transparent.Mode = TransparentModeTUN
	cfg.DevicePolicy.ProtectedIPv4 = []string{"192.168.50.253"}
	policy := device.PolicySet{
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices: []device.ManagedDevice{
			{ID: "phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.1.101", Profile: "home"},
			{ID: "laptop", MAC: "aa:bb:cc:dd:ee:02", IPv4: "192.168.50.101", Profile: "home", GatewayTarget: device.GatewayTargetUpstreamRouter},
		},
	}

	// Moving the Mac to a new LAN must not lock the operator out of the
	// configuration that still lists devices from the previous one. A dormant
	// router-bypass device must not impose bypass requirements on the new LAN.
	if err := ValidateDevicePolicyCandidate(cfg, policy); err != nil {
		t.Fatalf("ValidateDevicePolicyCandidate() error = %v", err)
	}
}

func TestValidateAcceptsUpstreamProxy(t *testing.T) {
	cfg := Default()
	cfg.UpstreamProxy.Enabled = true
	cfg.UpstreamProxy.Name = "real-device-egress"
	cfg.UpstreamProxy.Type = "http"
	cfg.UpstreamProxy.Server = "127.0.0.1"
	cfg.UpstreamProxy.Port = 18080
	cfg.UpstreamProxy.MatchDomain = "example.com"

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsImportedMihomoProfile(t *testing.T) {
	cfg := Default()
	cfg.Mihomo.ProfileMode = MihomoProfileModeImported
	cfg.Mihomo.Profile = "./profiles/home.yaml"
	cfg.Mihomo.ProfileSourceDigest = strings.Repeat("a", 64)
	cfg.Mihomo.ProfileOverlayDigest = strings.Repeat("b", 64)

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidMihomoProfileConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "unknown mode",
			edit: func(cfg *Config) {
				cfg.Mihomo.ProfileMode = "raw"
			},
			want: "mihomo.profile_mode must be managed or imported",
		},
		{
			name: "managed profile path",
			edit: func(cfg *Config) {
				cfg.Mihomo.Profile = "./profiles/home.yaml"
			},
			want: "mihomo.profile requires",
		},
		{
			name: "managed composition digest",
			edit: func(cfg *Config) {
				cfg.Mihomo.ProfileSourceDigest = strings.Repeat("a", 64)
			},
			want: "composition digests require",
		},
		{
			name: "imported missing profile path",
			edit: func(cfg *Config) {
				cfg.Mihomo.ProfileMode = MihomoProfileModeImported
			},
			want: "mihomo.profile is required",
		},
		{
			name: "uppercase source digest",
			edit: func(cfg *Config) {
				cfg.Mihomo.ProfileMode = MihomoProfileModeImported
				cfg.Mihomo.Profile = "./profiles/home.yaml"
				cfg.Mihomo.ProfileSourceDigest = strings.Repeat("A", 64)
			},
			want: "profile_source_digest must be a lowercase SHA-256 digest",
		},
		{
			name: "short overlay digest",
			edit: func(cfg *Config) {
				cfg.Mihomo.ProfileMode = MihomoProfileModeImported
				cfg.Mihomo.Profile = "./profiles/home.yaml"
				cfg.Mihomo.ProfileOverlayDigest = "abcd"
			},
			want: "profile_overlay_digest must be a lowercase SHA-256 digest",
		},
		{
			name: "imported with upstream proxy smoke",
			edit: func(cfg *Config) {
				cfg.Mihomo.ProfileMode = MihomoProfileModeImported
				cfg.Mihomo.Profile = "./profiles/home.yaml"
				cfg.UpstreamProxy.Enabled = true
			},
			want: "upstream_proxy.enabled cannot be true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.edit(&cfg)

			err := Validate(cfg)
			if err == nil {
				t.Fatalf("Validate() succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsInvalidUpstreamProxy(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "missing server",
			edit: func(cfg *Config) {
				cfg.UpstreamProxy.Server = ""
			},
			want: "upstream_proxy.server is required",
		},
		{
			name: "unsupported type",
			edit: func(cfg *Config) {
				cfg.UpstreamProxy.Type = "direct"
			},
			want: "upstream_proxy.type must be http or socks5",
		},
		{
			name: "invalid domain rule",
			edit: func(cfg *Config) {
				cfg.UpstreamProxy.MatchDomain = "https://example.com/"
			},
			want: "upstream_proxy.match_domain must be a domain",
		},
		{
			name: "invalid port",
			edit: func(cfg *Config) {
				cfg.UpstreamProxy.Port = 0
			},
			want: "upstream_proxy.port must be between 1 and 65535",
		},
		{
			name: "reserved local routing name",
			edit: func(cfg *Config) {
				cfg.UpstreamProxy.Name = "open-surge/mac-global"
			},
			want: "upstream_proxy.name must differ from reserved OpenSurge proxy groups",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.UpstreamProxy.Enabled = true
			cfg.UpstreamProxy.Name = "real-device-egress"
			cfg.UpstreamProxy.Type = "http"
			cfg.UpstreamProxy.Server = "127.0.0.1"
			cfg.UpstreamProxy.Port = 18080
			cfg.UpstreamProxy.MatchDomain = "example.com"
			tt.edit(&cfg)

			err := Validate(cfg)
			if err == nil {
				t.Fatalf("Validate() succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateDownstreamIPv6TakeoverContract(t *testing.T) {
	valid := Default()
	valid.Transparent.Mode = TransparentModeTUN
	valid.Transparent.TUNIPv6 = TUNIPv6Always
	if err := Validate(valid); err != nil {
		t.Fatalf("Validate(valid IPv6 takeover) error = %v", err)
	}

	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"same LAN without shared-L2 readiness", func(cfg *Config) {
			cfg.Gateway.Mode = GatewayModeSameLAN
			cfg.DHCP.Enabled = false
			cfg.Gateway.UpstreamInterface = cfg.Gateway.Interface
		}, "ipv6_shared_l2_ready"},
		{"transparent off", func(cfg *Config) { cfg.Transparent.Mode = TransparentModeOff }, "requires transparent.mode"},
		{"missing broker", func(cfg *Config) { cfg.Transparent.IPv6PacketBrokerBinary = "" }, "ipv6_packet_broker_binary is required"},
		{"MTU too small", func(cfg *Config) { cfg.Transparent.IPv6PacketMTU = 1279 }, "ipv6_packet_mtu must be between"},
		{"unknown mode", func(cfg *Config) { cfg.Transparent.TUNIPv6 = "sometimes" }, "tun_ipv6 must be off, auto, or always"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.edit(&cfg)
			if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}

	for _, mode := range []string{GatewayModeSameLAN, GatewayModeSameWiFiDHCP} {
		t.Run(mode+" with shared-L2 readiness", func(t *testing.T) {
			cfg := valid
			cfg.Gateway.Mode = mode
			cfg.Gateway.UpstreamInterface = cfg.Gateway.Interface
			cfg.Transparent.IPv6SharedL2Ready = true
			cfg.DHCP.Enabled = mode == GatewayModeSameWiFiDHCP
			if mode == GatewayModeSameWiFiDHCP {
				cfg.Gateway.LANIP = "192.168.50.1"
			}
			if err := Validate(cfg); err != nil {
				t.Fatalf("Validate(%s shared-L2 IPv6) error = %v", mode, err)
			}
		})
	}
}

func TestValidateTailscaleTargetsAndRouteConflicts(t *testing.T) {
	valid := Default()
	valid.Tailscale.Enabled = true
	valid.Tailscale.DisplayName = "Home Tailnet"
	valid.Tailscale.Hostname = "opensurge-home"
	valid.Tailscale.ControlURL = "https://controlplane.tailscale.com"
	valid.Tailscale.AuthKeyFile = "/tmp/tailscale-auth-key"
	valid.Tailscale.StateDir = "/tmp/tailscale-state"
	valid.Tailscale.AcceptRoutes = true
	valid.Tailscale.MagicDNSSuffixes = []string{"home.example.ts.net"}
	valid.Tailscale.PeerCIDRs = []string{"100.82.10.7/32"}
	valid.Tailscale.SubnetRoutes = []string{"10.20.0.0/16"}
	if err := Validate(valid); err != nil {
		t.Fatalf("Validate(valid Tailscale) error = %v", err)
	}

	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"LAN overlap", func(cfg *Config) { cfg.Tailscale.SubnetRoutes = []string{"192.168.50.0/24"} }, "overlaps the OpenSurge LAN"},
		{"route not accepted", func(cfg *Config) { cfg.Tailscale.AcceptRoutes = false }, "requires tailscale.accept_routes"},
		{"public subnet route", func(cfg *Config) { cfg.Tailscale.SubnetRoutes = []string{"203.0.113.0/24"} }, "must be a private"},
		{"wildcard suffix", func(cfg *Config) { cfg.Tailscale.MagicDNSSuffixes = []string{"*.example.ts.net"} }, "without wildcard"},
		{"noncanonical CIDR", func(cfg *Config) { cfg.Tailscale.PeerCIDRs = []string{"100.82.10.7/24"} }, "canonical IP CIDR"},
		{"broad Tailnet IPv4 capture", func(cfg *Config) { cfg.Tailscale.PeerCIDRs = []string{"100.64.0.0/10"} }, "one exact Tailscale peer"},
		{"broad Tailnet IPv6 capture", func(cfg *Config) { cfg.Tailscale.PeerCIDRs = []string{"fd7a:115c:a1e0::/48"} }, "one exact Tailscale peer"},
		{"LAN access without exit", func(cfg *Config) { cfg.Tailscale.ExitNodeAllowLANAccess = true }, "requires tailscale.exit_node"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.edit(&cfg)
			if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateTailscaleDeviceAccessUsesCurrentLAN(t *testing.T) {
	cfg := Default()
	cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
	cfg.Transparent.Mode = TransparentModeTUN
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.AllowedDevices = []string{"phone"}
	cfg.DevicePolicy.File = "already-loaded.json"
	scope, err := cfg.LANScope()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := device.CompilePolicyBundleForLAN(device.PolicySet{
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices:  []device.ManagedDevice{{ID: "phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.50.101", Profile: "home"}},
	}, scope, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DevicePolicy.Bundle = &bundle
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate(active device) error = %v", err)
	}

	cfg.Gateway.LANIP = "192.168.60.1"
	cfg.DHCP.RangeStart = "192.168.60.100"
	cfg.DHCP.RangeEnd = "192.168.60.200"
	cfg.DNS.Listen = cfg.Gateway.LANIP
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), `unknown or inactive device "phone"`) {
		t.Fatalf("Validate(device outside new LAN) error = %v", err)
	}
	if bundle.ActiveLAN != "192.168.50.0/24" || len(bundle.Compiled.Devices) != 1 {
		t.Fatal("validation mutated the caller's policy snapshot")
	}
}

func TestValidateRuntimeDoesNotLoadTailscaleDevicePolicy(t *testing.T) {
	cfg := Default()
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.AllowAllDevices = true
	cfg.DevicePolicy.File = filepath.Join(t.TempDir(), "missing-policy.json")
	if err := ValidateRuntime(cfg); err != nil {
		t.Fatalf("ValidateRuntime() must not depend on the desired policy file: %v", err)
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), cfg.DevicePolicy.File) {
		t.Fatalf("Validate() must report the missing policy file: %v", err)
	}
}
