package config

import (
	"strings"
	"testing"
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
			name: "imported missing profile path",
			edit: func(cfg *Config) {
				cfg.Mihomo.ProfileMode = MihomoProfileModeImported
			},
			want: "mihomo.profile is required",
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
