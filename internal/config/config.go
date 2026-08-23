package config

import "open-mihomo-gateway/internal/device"

import (
	"fmt"
	"net"
	"path/filepath"

	"open-mihomo-gateway/internal/lan"
)

type Config struct {
	Gateway          GatewayConfig
	DHCP             DHCPConfig
	DevicePolicy     DevicePolicyConfig
	DNS              DNSConfig
	Mihomo           MihomoConfig
	PF               PFConfig
	Transparent      TransparentConfig
	LocalSystemProxy LocalSystemProxyConfig
	UpstreamProxy    UpstreamProxyConfig
	Runtime          RuntimeConfig
}

// DevicePolicyConfig points at the optional JSON control-plane file that
// defines DHCP reservations and per-device mihomo policy overlays.
type DevicePolicyConfig struct {
	File          string
	ProtectedIPv4 []string
	Bundle        *device.PolicyBundle
}

type GatewayConfig struct {
	Mode      string
	Interface string
	LANIP     string
	// LANPrefixLen is the real subnet prefix of the downstream LAN. Zero means
	// the historical /24 assumption, so configs written before this field
	// existed keep behaving the same way.
	LANPrefixLen      int
	UpstreamInterface string
}

type DHCPConfig struct {
	Binary        string
	Enabled       bool
	RangeStart    string
	RangeEnd      string
	LeaseTime     string
	Domain        string
	BypassGateway string
	BypassDNS     []string
}

type DNSConfig struct {
	Listen   string
	Port     int
	Upstream string
	IPv6     bool
}

type MihomoConfig struct {
	Binary               string
	Config               string
	ProfileMode          string
	Profile              string
	ProfileSourceDigest  string
	ProfileOverlayDigest string
	StoreFakeIP          bool
	MixedPort            int
	RedirPort            int
	APIAddr              string
	Secret               string
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
	TUNIPv6Off    = "off"
	TUNIPv6Auto   = "auto"
	TUNIPv6Always = "always"
)

// The three IPv6 ranges are deliberately disjoint. Downstream clients use the
// LAN /64, fake-IP answers use another /64 so clients do not attempt on-link
// neighbour discovery for synthetic destinations, and the host TUN keeps its
// own tiny point-to-point range.
const (
	MihomoFakeIPv6Range        = "fdfe:dcba:9876::/64"
	MihomoTUNIPv6              = "fdfe:dcba:9877::1/126"
	DownstreamIPv6Prefix       = "fdfe:dcba:9878::/64"
	DownstreamIPv6Gateway      = "fdfe:dcba:9878::1"
	IPv6PacketListenerName     = "opensurge-ipv6"
	IPv6PacketBrokerSubcommand = "ipv6-packet"
)

// MihomoDNSUpstream is the dnsmasq upstream that preserves mihomo fake-IP and
// TUN DNS semantics. An explicit public resolver remains supported for
// diagnostics, but TUN dns-hijack means it is not a guaranteed bypass path.
const MihomoDNSUpstream = "127.0.0.1#1053"

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
	TUNIPv6                string
	IPv6SharedL2Ready      bool
	IPv6PacketBrokerBinary string
	IPv6PacketMTU          int
}

// LocalSystemProxyConfig enables an opt-in compatibility layer for local Mac
// applications that honor the macOS HTTP/HTTPS proxy settings. The endpoint
// is derived from Mihomo.MixedPort and is not independently configurable.
type LocalSystemProxyConfig struct {
	Enabled bool
}

func (c TransparentConfig) TUNEnabled() bool {
	return c.Mode == TransparentModeTUN
}

func (c TransparentConfig) IPv6Requested() bool {
	return c.TUNIPv6 != TUNIPv6Off
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
			LANPrefixLen:      lan.DefaultPrefixLen,
			UpstreamInterface: "en0",
		},
		DHCP: DHCPConfig{
			Binary:     "dnsmasq",
			Enabled:    true,
			RangeStart: "192.168.50.100",
			RangeEnd:   "192.168.50.200",
			LeaseTime:  "12h",
			Domain:     "lan",
			BypassDNS:  []string{},
		},
		DevicePolicy: DevicePolicyConfig{},
		DNS: DNSConfig{
			Listen:   "192.168.50.1",
			Port:     53,
			Upstream: MihomoDNSUpstream,
			IPv6:     false,
		},
		Mihomo: MihomoConfig{
			Binary:      "./bin/mihomo",
			Config:      "./runtime/mihomo.yaml",
			ProfileMode: MihomoProfileModeManaged,
			Profile:     "",
			StoreFakeIP: true,
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
			TUNIPv6:                TUNIPv6Off,
			IPv6SharedL2Ready:      false,
			IPv6PacketBrokerBinary: "opensurge-network",
			IPv6PacketMTU:          1500,
		},
		LocalSystemProxy: LocalSystemProxyConfig{Enabled: false},
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

// LANScope resolves the downstream LAN from gateway.lan_ip and
// gateway.lan_prefix_len. Every subnet decision goes through it so pf NAT,
// mihomo route exclusion, DHCP ranges, and device addresses cannot disagree.
func (c Config) LANScope() (lan.Scope, error) {
	scope, err := lan.NewScope(c.Gateway.LANIP, c.Gateway.LANPrefixLen)
	if err != nil {
		return lan.Scope{}, fmt.Errorf("gateway.lan_ip / gateway.lan_prefix_len: %w", err)
	}
	return scope, nil
}

func (c Config) LANPrefix() (string, error) {
	scope, err := c.LANScope()
	if err != nil {
		return "", err
	}
	return scope.String(), nil
}
