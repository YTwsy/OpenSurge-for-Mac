package config

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// ExternalDNSFilterConfig is applied with the gateway configuration, not a source draft.
type ExternalDNSFilterConfig struct {
	Enabled          bool     `json:"enabled"`
	Server           string   `json:"server"`
	TimeoutMS        int      `json:"timeout_ms"`
	BlockedAddresses []string `json:"blocked_addresses"`
	BypassDomains    []string `json:"bypass_domains"`
}

func ValidateExternalDNSFilter(cfg Config) error {
	c := cfg.ExternalDNSFilter
	if !c.Enabled {
		return nil
	}
	ap, err := netip.ParseAddrPort(c.Server)
	if err != nil || ap.Port() == 0 || ap.Addr().Zone() != "" || ap.Addr().IsUnspecified() || ap.Addr().IsMulticast() {
		return fmt.Errorf("external_dns_filter.server must be a literal IPv4:port or [IPv6]:port")
	}
	ip := ap.Addr().Unmap()
	if netip.MustParsePrefix("198.18.0.0/16").Contains(ip) || netip.MustParsePrefix(MihomoFakeIPv6Range).Contains(ip) {
		return fmt.Errorf("external_dns_filter.server cannot be a Fake-IP address")
	}
	if int(ap.Port()) == cfg.DNS.Port || ap.Port() == 1053 {
		local := ip.IsLoopback() || ip.String() == cfg.DNS.Listen || ip.String() == cfg.Gateway.LANIP || ip.String() == DownstreamIPv6Gateway
		addresses, err := net.InterfaceAddrs()
		if err != nil {
			return fmt.Errorf("check external DNS filter listener loop: %w", err)
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err == nil && prefix.Addr().Unmap() == ip {
				local = true
			}
		}
		if local {
			return fmt.Errorf("external_dns_filter.server cannot point to OpenSurge DNS listeners")
		}
	}
	if c.TimeoutMS < 100 || c.TimeoutMS > 10000 {
		return fmt.Errorf("external_dns_filter.timeout_ms must be between 100 and 10000")
	}
	if len(c.BlockedAddresses) > 64 || len(c.BypassDomains) > 128 {
		return fmt.Errorf("external DNS filter allows at most 64 blocked addresses and 128 bypass domains")
	}
	for _, value := range c.BlockedAddresses {
		address, err := netip.ParseAddr(value)
		if err != nil || address.Zone() != "" {
			return fmt.Errorf("external_dns_filter.blocked_addresses must contain literal IP addresses")
		}
	}
	for _, value := range c.BypassDomains {
		value = strings.TrimSuffix(value, ".")
		if value == "" || len(value) > 253 {
			return fmt.Errorf("invalid external_dns_filter.bypass_domains suffix")
		}
		for _, label := range strings.Split(value, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return fmt.Errorf("invalid external_dns_filter.bypass_domains label")
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
					return fmt.Errorf("external_dns_filter.bypass_domains accepts ASCII domain suffixes without wildcards")
				}
			}
		}
	}
	if cfg.DNS.Upstream != "" && cfg.DNS.Upstream != MihomoDNSUpstream {
		return fmt.Errorf("external DNS filtering requires dns.upstream: \"127.0.0.1#1053\"")
	}
	return nil
}
