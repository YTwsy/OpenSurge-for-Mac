package config

import "open-mihomo-gateway/internal/device"

import (
	"fmt"
	"net"
	"path/filepath"
)

type Config struct {
	Gateway       GatewayConfig
	DHCP          DHCPConfig
	DevicePolicy  DevicePolicyConfig
	DNS           DNSConfig
	Mihomo        MihomoConfig
	PF            PFConfig
	Transparent   TransparentConfig
	UpstreamProxy UpstreamProxyConfig
	Runtime       RuntimeConfig
}

// DevicePolicyConfig points at the optional JSON control-plane file that
// defines DHCP reservations and per-device mihomo policy overlays.
type DevicePolicyConfig struct {
	File          string
	ProtectedIPv4 []string
	Bundle        *device.PolicyBundle
}

type GatewayConfig struct {
	Mode              string
	Interface         string
	LANIP             string
	UpstreamInterface string
}

type DHCPConfig struct {
	Binary     string
	Enabled    bool
	RangeStart string
	RangeEnd   string
	LeaseTime  string
	Domain     string
}

type DNSConfig struct {
	Listen   string
	Port     int
	Upstream string
}

type MihomoConfig struct {
	Binary      string
	Config      string
	ProfileMode string
	Profile     string
	MixedPort   int
	RedirPort   int
	APIAddr     string
	Secret      string
}

type PFConfig struct {
	AnchorName    string
	RedirectTCPTo int
}

const (
	GatewayModeIsolatedLAN  = "isolated_lan"
	GatewayModeSameLAN      = "same_lan"
	GatewayModeSameWiFiDHCP = "same_wifi_dhcp"
)

const (
	TransparentModeOff = "off"
	TransparentModeTUN = "tun"
)

const (
	MihomoProfileModeManaged  = "managed"
	MihomoProfileModeImported = "imported"
)

type TransparentConfig struct {
	Mode                   string
	TUNDevice              string
	TUNStack               string
	TUNAutoRoute           bool
	TUNAutoDetectInterface bool
	TUNStrictRoute         bool
}

func (c TransparentConfig) TUNEnabled() bool {
	return c.Mode == TransparentModeTUN
}

func (c GatewayConfig) SameLAN() bool {
	return c.Mode == GatewayModeSameLAN || c.Mode == GatewayModeSameWiFiDHCP
}

type UpstreamProxyConfig struct {
	Enabled     bool
	Name        string
	Type        string
	Server      string
	Port        int
	Username    string
	Password    string
	MatchDomain string
}

type RuntimeConfig struct {
	Dir string
}

func Default() Config {
	return Config{
		Gateway: GatewayConfig{
			Mode:              GatewayModeIsolatedLAN,
			Interface:         "en0",
			LANIP:             "192.168.50.1",
			UpstreamInterface: "en0",
		},
		DHCP: DHCPConfig{
			Binary:     "dnsmasq",
			Enabled:    true,
			RangeStart: "192.168.50.100",
			RangeEnd:   "192.168.50.200",
			LeaseTime:  "12h",
			Domain:     "lan",
		},
		DevicePolicy: DevicePolicyConfig{},
		DNS: DNSConfig{
			Listen: "192.168.50.1",
			Port:   53,
		},
		Mihomo: MihomoConfig{
			Binary:      "./bin/mihomo",
			Config:      "./runtime/mihomo.yaml",
			ProfileMode: MihomoProfileModeManaged,
			Profile:     "",
			MixedPort:   7890,
			RedirPort:   0,
			APIAddr:     "127.0.0.1:9090",
			Secret:      "",
		},
		PF: PFConfig{
			AnchorName:    "com.apple/open_mihomo_gateway",
			RedirectTCPTo: 0,
		},
		Transparent: TransparentConfig{
			Mode:                   TransparentModeOff,
			TUNDevice:              "utun123",
			TUNStack:               "mixed",
			TUNAutoRoute:           true,
			TUNAutoDetectInterface: false,
			TUNStrictRoute:         false,
		},
		UpstreamProxy: UpstreamProxyConfig{
			Enabled:     false,
			Name:        "real-device-egress",
			Type:        "http",
			Server:      "127.0.0.1",
			Port:        0,
			MatchDomain: "example.com",
		},
		Runtime: RuntimeConfig{
			Dir: "./runtime",
		},
	}
}

func (c Config) LANIP() net.IP {
	return net.ParseIP(c.Gateway.LANIP)
}

func (c Config) RuntimePath(name string) string {
	return filepath.Join(c.Runtime.Dir, name)
}

func (c Config) LANPrefix24() (string, error) {
	ip := c.LANIP().To4()
	if ip == nil {
		return "", fmt.Errorf("gateway.lan_ip must be an IPv4 address")
	}
	return fmt.Sprintf("%d.%d.%d.0/24", ip[0], ip[1], ip[2]), nil
}
