package config

import (
	"fmt"
	"strings"

	"open-mihomo-gateway/internal/lan"
)

// Render serializes the complete supported gateway configuration. It is used
// by the GUI control plane so config edits never depend on preserving comments
// or unknown YAML fields that the loader would reject anyway.
func Render(cfg Config) string {
	q := func(value string) string { return fmt.Sprintf("%q", value) }
	return fmt.Sprintf(`gateway:
  mode: %s
  interface: %s
  lan_ip: %s
  lan_prefix_len: %d
  upstream_interface: %s

dhcp:
  binary: %s
  enabled: %t
  range_start: %s
  range_end: %s
  lease_time: %s
  domain: %s
  bypass_gateway: %s
  bypass_dns: %s

device_policy:
  file: %s
  protected_ipv4: %s

dns:
  listen: %s
  port: %d
  upstream: %s
  ipv6: %t

mihomo:
  binary: %s
  config: %s
  profile_mode: %s
  profile: %s
  store_fake_ip: %t
  mixed_port: %d
  redir_port: %d
  api_addr: %s
  secret: %s

tailscale:
  enabled: %t
  display_name: %s
  hostname: %s
  control_url: %s
  auth_key_file: %s
  state_dir: %s
  accept_routes: %t
  magic_dns_suffixes: %s
  peer_cidrs: %s
  subnet_routes: %s
  allow_mac: %t
  allow_all_devices: %t
  allowed_devices: %s
  exit_node: %s
  exit_node_allow_lan_access: %t

pf:
  anchor_name: %s
  redirect_tcp_to: %d

transparent:
  mode: %s
  tun_device: %s
  tun_stack: %s
  tun_auto_route: %t
  tun_auto_detect_interface: %t
  tun_strict_route: %t
  tun_ipv6: %s
  ipv6_shared_l2_ready: %t
  ipv6_packet_broker_binary: %s
  ipv6_packet_mtu: %d

local_system_proxy:
  enabled: %t

upstream_proxy:
  enabled: %t
  name: %s
  type: %s
  server: %s
  port: %d
  username: %s
  password: %s
  match_domain: %s

runtime:
  dir: %s
`,
		q(cfg.Gateway.Mode), q(cfg.Gateway.Interface), q(cfg.Gateway.LANIP), lan.PrefixLenOrDefault(cfg.Gateway.LANPrefixLen), q(cfg.Gateway.UpstreamInterface),
		q(cfg.DHCP.Binary), cfg.DHCP.Enabled, q(cfg.DHCP.RangeStart), q(cfg.DHCP.RangeEnd), q(cfg.DHCP.LeaseTime), q(cfg.DHCP.Domain), q(cfg.DHCP.BypassGateway), q(strings.Join(cfg.DHCP.BypassDNS, ",")),
		q(cfg.DevicePolicy.File), q(strings.Join(cfg.DevicePolicy.ProtectedIPv4, ",")),
		q(cfg.DNS.Listen), cfg.DNS.Port, q(cfg.DNS.Upstream), cfg.DNS.IPv6,
		q(cfg.Mihomo.Binary), q(cfg.Mihomo.Config), q(cfg.Mihomo.ProfileMode), q(cfg.Mihomo.Profile), cfg.Mihomo.StoreFakeIP, cfg.Mihomo.MixedPort, cfg.Mihomo.RedirPort, q(cfg.Mihomo.APIAddr), q(cfg.Mihomo.Secret),
		cfg.Tailscale.Enabled, q(cfg.Tailscale.DisplayName), q(cfg.Tailscale.Hostname), q(cfg.Tailscale.ControlURL), q(cfg.Tailscale.AuthKeyFile), q(cfg.Tailscale.StateDir), cfg.Tailscale.AcceptRoutes,
		q(strings.Join(cfg.Tailscale.MagicDNSSuffixes, ",")), q(strings.Join(cfg.Tailscale.PeerCIDRs, ",")), q(strings.Join(cfg.Tailscale.SubnetRoutes, ",")), cfg.Tailscale.AllowMac, cfg.Tailscale.AllowAllDevices,
		q(strings.Join(cfg.Tailscale.AllowedDevices, ",")), q(cfg.Tailscale.ExitNode), cfg.Tailscale.ExitNodeAllowLANAccess,
		q(cfg.PF.AnchorName), cfg.PF.RedirectTCPTo,
		q(cfg.Transparent.Mode), q(cfg.Transparent.TUNDevice), q(cfg.Transparent.TUNStack), cfg.Transparent.TUNAutoRoute, cfg.Transparent.TUNAutoDetectInterface, cfg.Transparent.TUNStrictRoute,
		q(cfg.Transparent.TUNIPv6), cfg.Transparent.IPv6SharedL2Ready, q(cfg.Transparent.IPv6PacketBrokerBinary), cfg.Transparent.IPv6PacketMTU,
		cfg.LocalSystemProxy.Enabled,
		cfg.UpstreamProxy.Enabled, q(cfg.UpstreamProxy.Name), q(cfg.UpstreamProxy.Type), q(cfg.UpstreamProxy.Server), cfg.UpstreamProxy.Port, q(cfg.UpstreamProxy.Username), q(cfg.UpstreamProxy.Password), q(cfg.UpstreamProxy.MatchDomain),
		q(cfg.Runtime.Dir),
	)
}
