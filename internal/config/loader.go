package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func Load(path string) (Config, error) {
	return load(path, true)
}

// LoadRuntime parses and validates the gateway configuration while deliberately
// deferring the mutable desired device-policy document. It lets stop/status and
// applied-policy controls continue to work when an operator is editing an
// invalid next policy on disk.
func LoadRuntime(path string) (Config, error) {
	return load(path, false)
}

func load(path string, loadDevicePolicy bool) (Config, error) {
	cfg := Default()

	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		raw := strings.TrimSpace(stripComment(scanner.Text()))
		if raw == "" {
			continue
		}
		if strings.HasSuffix(raw, ":") {
			section = strings.TrimSuffix(raw, ":")
			continue
		}

		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			return Config{}, fmt.Errorf("%s:%d: expected key: value", path, lineNo)
		}
		key = strings.TrimSpace(key)
		value = unquote(strings.TrimSpace(value))
		if err := applyValue(&cfg, section, key, value); err != nil {
			return Config{}, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	resolveRelativePaths(path, &cfg)
	if loadDevicePolicy {
		if err := PrepareDevicePolicy(&cfg); err != nil {
			return Config{}, err
		}
	}
	if err := validate(cfg, loadDevicePolicy); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func resolveRelativePaths(configPath string, cfg *Config) {
	if cfg.Mihomo.Profile != "" && !filepath.IsAbs(cfg.Mihomo.Profile) {
		cfg.Mihomo.Profile = filepath.Join(filepath.Dir(configPath), cfg.Mihomo.Profile)
	}
	if cfg.DevicePolicy.File != "" && !filepath.IsAbs(cfg.DevicePolicy.File) {
		cfg.DevicePolicy.File = filepath.Join(filepath.Dir(configPath), cfg.DevicePolicy.File)
	}
}

func stripComment(line string) string {
	inSingle := false
	inDouble := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func applyValue(cfg *Config, section, key, value string) error {
	switch section + "." + key {
	case "gateway.mode":
		cfg.Gateway.Mode = strings.ToLower(value)
	case "gateway.interface":
		cfg.Gateway.Interface = value
	case "gateway.lan_ip":
		cfg.Gateway.LANIP = value
	case "gateway.upstream_interface":
		cfg.Gateway.UpstreamInterface = value
	case "dhcp.binary":
		cfg.DHCP.Binary = value
	case "dhcp.enabled":
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("dhcp.enabled must be a boolean")
		}
		cfg.DHCP.Enabled = enabled
	case "dhcp.range_start":
		cfg.DHCP.RangeStart = value
	case "dhcp.range_end":
		cfg.DHCP.RangeEnd = value
	case "dhcp.lease_time":
		cfg.DHCP.LeaseTime = value
	case "dhcp.domain":
		cfg.DHCP.Domain = value
	case "device_policy.file":
		cfg.DevicePolicy.File = value
	case "device_policy.protected_ipv4":
		cfg.DevicePolicy.ProtectedIPv4 = splitCommaSeparated(value)
	case "dns.listen":
		cfg.DNS.Listen = value
	case "dns.port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("dns.port must be a number")
		}
		cfg.DNS.Port = port
	case "dns.upstream":
		cfg.DNS.Upstream = value
	case "mihomo.binary":
		cfg.Mihomo.Binary = value
	case "mihomo.config":
		cfg.Mihomo.Config = value
	case "mihomo.profile_mode":
		cfg.Mihomo.ProfileMode = strings.ToLower(value)
	case "mihomo.profile":
		cfg.Mihomo.Profile = value
	case "mihomo.mixed_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("mihomo.mixed_port must be a number")
		}
		cfg.Mihomo.MixedPort = port
	case "mihomo.redir_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("mihomo.redir_port must be a number")
		}
		cfg.Mihomo.RedirPort = port
	case "mihomo.api_addr":
		cfg.Mihomo.APIAddr = value
	case "mihomo.secret":
		cfg.Mihomo.Secret = value
	case "pf.anchor_name":
		cfg.PF.AnchorName = value
	case "pf.redirect_tcp_to":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("pf.redirect_tcp_to must be a number")
		}
		cfg.PF.RedirectTCPTo = port
	case "transparent.mode":
		cfg.Transparent.Mode = strings.ToLower(value)
	case "transparent.tun_device":
		cfg.Transparent.TUNDevice = value
	case "transparent.tun_stack":
		cfg.Transparent.TUNStack = strings.ToLower(value)
	case "transparent.tun_auto_route":
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("transparent.tun_auto_route must be a boolean")
		}
		cfg.Transparent.TUNAutoRoute = enabled
	case "transparent.tun_auto_detect_interface":
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("transparent.tun_auto_detect_interface must be a boolean")
		}
		cfg.Transparent.TUNAutoDetectInterface = enabled
	case "transparent.tun_strict_route":
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("transparent.tun_strict_route must be a boolean")
		}
		cfg.Transparent.TUNStrictRoute = enabled
	case "upstream_proxy.enabled":
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("upstream_proxy.enabled must be a boolean")
		}
		cfg.UpstreamProxy.Enabled = enabled
	case "upstream_proxy.name":
		cfg.UpstreamProxy.Name = value
	case "upstream_proxy.type":
		cfg.UpstreamProxy.Type = strings.ToLower(value)
	case "upstream_proxy.server":
		cfg.UpstreamProxy.Server = value
	case "upstream_proxy.port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("upstream_proxy.port must be a number")
		}
		cfg.UpstreamProxy.Port = port
	case "upstream_proxy.username":
		cfg.UpstreamProxy.Username = value
	case "upstream_proxy.password":
		cfg.UpstreamProxy.Password = value
	case "upstream_proxy.match_domain":
		cfg.UpstreamProxy.MatchDomain = strings.ToLower(value)
	case "runtime.dir":
		cfg.Runtime.Dir = value
	default:
		return fmt.Errorf("unknown config key %s.%s", section, key)
	}
	return nil
}

func splitCommaSeparated(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
