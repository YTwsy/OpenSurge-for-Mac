package config

import (
	"fmt"
	"net"
	"path/filepath"
)

type Config struct {
	Gateway GatewayConfig
	DHCP    DHCPConfig
	DNS     DNSConfig
	Mihomo  MihomoConfig
	PF      PFConfig
	Runtime RuntimeConfig
}

type GatewayConfig struct {
	Interface         string
	LANIP             string
	UpstreamInterface string
}

type DHCPConfig struct {
	Enabled    bool
	RangeStart string
	RangeEnd   string
	LeaseTime  string
	Domain     string
}

type DNSConfig struct {
	Listen string
	Port   int
}

type MihomoConfig struct {
	Binary    string
	Config    string
	MixedPort int
	RedirPort int
	APIAddr   string
	Secret    string
}

type PFConfig struct {
	AnchorName    string
	RedirectTCPTo int
}

type RuntimeConfig struct {
	Dir string
}

func Default() Config {
	return Config{
		Gateway: GatewayConfig{
			Interface:         "en0",
			LANIP:             "192.168.50.1",
			UpstreamInterface: "en0",
		},
		DHCP: DHCPConfig{
			Enabled:    true,
			RangeStart: "192.168.50.100",
			RangeEnd:   "192.168.50.200",
			LeaseTime:  "12h",
			Domain:     "lan",
		},
		DNS: DNSConfig{
			Listen: "192.168.50.1",
			Port:   53,
		},
		Mihomo: MihomoConfig{
			Binary:    "./bin/mihomo",
			Config:    "./runtime/mihomo.yaml",
			MixedPort: 7890,
			RedirPort: 7892,
			APIAddr:   "127.0.0.1:9090",
			Secret:    "",
		},
		PF: PFConfig{
			AnchorName:    "open_mihomo_gateway",
			RedirectTCPTo: 7892,
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
