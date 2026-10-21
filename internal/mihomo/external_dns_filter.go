package mihomo

import (
	"net/netip"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

func renderExternalDNSFilter(cfg config.Config) (string, error) {
	if err := config.ValidateExternalDNSFilter(cfg); err != nil {
		return "", err
	}
	if !cfg.ExternalDNSFilter.Enabled {
		return "", nil
	}
	c := cfg.ExternalDNSFilter
	endpoint, _ := netip.ParseAddrPort(c.Server)
	filterInterface := cfg.Gateway.UpstreamInterface
	if scope, err := cfg.LANScope(); err == nil && scope.Network.Contains(endpoint.Addr().Unmap().AsSlice()) {
		filterInterface = cfg.Gateway.Interface
	}
	if endpoint.Addr().IsLoopback() || !cfg.Transparent.TUNEnabled() {
		filterInterface = ""
	}
	bypass := append([]string{}, c.BypassDomains...)
	if (cfg.DHCP.Enabled || cfg.ManageSystemDNS()) && cfg.DHCP.Domain != "" {
		bypass = append(bypass, cfg.DHCP.Domain)
	}
	if cfg.ManageSystemDNS() {
		for _, domain := range cfg.LocalSystemDNS.Domains {
			if domain = strings.Trim(strings.ToLower(domain), ". "); domain != "" {
				bypass = append(bypass, domain)
			}
		}
	}
	if cfg.Tailscale.Enabled {
		bypass = append(bypass, cfg.Tailscale.MagicDNSSuffixes...)
	}
	data, err := yaml.Marshal(map[string]any{"opensurge-external-filter": map[string]any{
		"enabled": true, "server": c.Server, "timeout-ms": c.TimeoutMS, "interface": filterInterface,
		"blocked-addresses": append([]string{}, c.BlockedAddresses...), "bypass-domains": bypass,
	}})
	return indentYAMLBlock(strings.TrimSpace(string(data)), "  "), err
}

// An unsupported binary must fail both validation and startup, including the
// prepared workspace. Otherwise upstream YAML parsing silently ignores the hook.
func engineConfigArgs(directory, path string, validate bool) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		DNS struct {
			Filter struct {
				Enabled bool `yaml:"enabled"`
			} `yaml:"opensurge-external-filter"`
		} `yaml:"dns"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	args := []string{"-d", directory}
	if validate {
		args = append(args, "-t")
	}
	args = append(args, "-f", path)
	if cfg.DNS.Filter.Enabled {
		args = append(args, "-require-opensurge-dns-filter")
	}
	return args, nil
}
