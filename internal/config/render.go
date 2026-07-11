package config

import (
	"fmt"
	"strings"
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
  upstream_interface: %s

dhcp:
  binary: %s
  enabled: %t
  range_start: %s
  range_end: %s
  lease_time: %s
  domain: %s

device_policy:
  file: %s
  protected_ipv4: %s

dns:
  listen: %s
  port: %d
  upstream: %s

mihomo:
  binary: %s
  config: %s
  profile_mode: %s
  profile: %s
  mixed_port: %d
  redir_port: %d
  api_addr: %s
  secret: %s

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
		q(cfg.Gateway.Mode), q(cfg.Gateway.Interface), q(cfg.Gateway.LANIP), q(cfg.Gateway.UpstreamInterface),
		q(cfg.DHCP.Binary), cfg.DHCP.Enabled, q(cfg.DHCP.RangeStart), q(cfg.DHCP.RangeEnd), q(cfg.DHCP.LeaseTime), q(cfg.DHCP.Domain),
		q(cfg.DevicePolicy.File), q(strings.Join(cfg.DevicePolicy.ProtectedIPv4, ",")),
		q(cfg.DNS.Listen), cfg.DNS.Port, q(cfg.DNS.Upstream),
		q(cfg.Mihomo.Binary), q(cfg.Mihomo.Config), q(cfg.Mihomo.ProfileMode), q(cfg.Mihomo.Profile), cfg.Mihomo.MixedPort, cfg.Mihomo.RedirPort, q(cfg.Mihomo.APIAddr), q(cfg.Mihomo.Secret),
		q(cfg.PF.AnchorName), cfg.PF.RedirectTCPTo,
		q(cfg.Transparent.Mode), q(cfg.Transparent.TUNDevice), q(cfg.Transparent.TUNStack), cfg.Transparent.TUNAutoRoute, cfg.Transparent.TUNAutoDetectInterface, cfg.Transparent.TUNStrictRoute,
		cfg.UpstreamProxy.Enabled, q(cfg.UpstreamProxy.Name), q(cfg.UpstreamProxy.Type), q(cfg.UpstreamProxy.Server), cfg.UpstreamProxy.Port, q(cfg.UpstreamProxy.Username), q(cfg.UpstreamProxy.Password), q(cfg.UpstreamProxy.MatchDomain),
		q(cfg.Runtime.Dir),
	)
}
