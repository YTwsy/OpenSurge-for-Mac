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
