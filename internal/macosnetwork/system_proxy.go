package macosnetwork

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"open-mihomo-gateway/internal/runtime"
)

const localSystemProxyHost = "127.0.0.1"

// SystemProxy manages only the HTTP and HTTPS proxy settings for the network
// service associated with the configured upstream interface. SOCKS, PAC,
// auto-discovery, and bypass domains remain outside its write scope.
type SystemProxy struct{}

func (SystemProxy) Prepare(ctx context.Context, interfaceName string, port int) (runtime.SystemProxySnapshot, error) {
	if strings.TrimSpace(interfaceName) == "" {
		return runtime.SystemProxySnapshot{}, fmt.Errorf("upstream interface is required")
	}
	if port < 1 || port > 65535 {
		return runtime.SystemProxySnapshot{}, fmt.Errorf("local system proxy port must be between 1 and 65535")
	}
	service, err := NetworkServiceForInterface(ctx, interfaceName)
	if err != nil {
		return runtime.SystemProxySnapshot{}, err
	}
	httpSetting, err := readSystemProxySetting(ctx, "-getwebproxy", service)
	if err != nil {
		return runtime.SystemProxySnapshot{}, err
	}
	httpsSetting, err := readSystemProxySetting(ctx, "-getsecurewebproxy", service)
	if err != nil {
		return runtime.SystemProxySnapshot{}, err
	}
	autoConfigEnabled, err := readProxyFeatureEnabled(ctx, "-getautoproxyurl", service)
	if err != nil {
		return runtime.SystemProxySnapshot{}, err
	}
	autoDiscoveryEnabled, err := readProxyFeatureEnabled(ctx, "-getproxyautodiscovery", service)
	if err != nil {
		return runtime.SystemProxySnapshot{}, err
	}
	snapshot := runtime.SystemProxySnapshot{
		NetworkService:       service,
		Interface:            interfaceName,
		HTTP:                 httpSetting,
		HTTPS:                httpsSetting,
		AutoConfigEnabled:    autoConfigEnabled,
		AutoDiscoveryEnabled: autoDiscoveryEnabled,
	}
	if httpSetting.Authenticated || httpsSetting.Authenticated {
		return runtime.SystemProxySnapshot{}, fmt.Errorf("network service %q has authenticated HTTP/HTTPS proxy settings that OpenSurge cannot restore without credentials", service)
	}
	if httpSetting.Enabled || httpsSetting.Enabled {
		return runtime.SystemProxySnapshot{}, fmt.Errorf("network service %q already has an active HTTP/HTTPS system proxy; disable it before enabling OpenSurge coordination", service)
	}
	if autoConfigEnabled || autoDiscoveryEnabled {
		return runtime.SystemProxySnapshot{}, fmt.Errorf("network service %q has active proxy auto-configuration or discovery; disable it before enabling OpenSurge coordination", service)
	}
	return snapshot, nil
}

func (SystemProxy) Enable(ctx context.Context, snapshot runtime.SystemProxySnapshot, port int) error {
	if strings.TrimSpace(snapshot.NetworkService) == "" {
		return fmt.Errorf("system proxy snapshot is missing its network service")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("local system proxy port must be between 1 and 65535")
	}
	if err := setSystemProxyEndpoint(ctx, "-setwebproxy", snapshot.NetworkService, localSystemProxyHost, port); err != nil {
		return err
	}
	if err := setSystemProxyEndpoint(ctx, "-setsecurewebproxy", snapshot.NetworkService, localSystemProxyHost, port); err != nil {
		restoreErr := (SystemProxy{}).Restore(ctx, snapshot)
		return errors.Join(err, restoreErr)
	}
	if err := verifySystemProxyEndpoint(ctx, "-getwebproxy", snapshot.NetworkService, port); err != nil {
		restoreErr := (SystemProxy{}).Restore(ctx, snapshot)
		return errors.Join(err, restoreErr)
	}
	if err := verifySystemProxyEndpoint(ctx, "-getsecurewebproxy", snapshot.NetworkService, port); err != nil {
		restoreErr := (SystemProxy{}).Restore(ctx, snapshot)
		return errors.Join(err, restoreErr)
	}
	return nil
}

func (SystemProxy) Restore(ctx context.Context, snapshot runtime.SystemProxySnapshot) error {
	if strings.TrimSpace(snapshot.NetworkService) == "" {
		return fmt.Errorf("system proxy snapshot is missing its network service")
	}
	var restoreErr error
	restoreErr = errors.Join(restoreErr, restoreSystemProxySetting(ctx, "-setwebproxy", "-setwebproxystate", snapshot.NetworkService, snapshot.HTTP))
	restoreErr = errors.Join(restoreErr, restoreSystemProxySetting(ctx, "-setsecurewebproxy", "-setsecurewebproxystate", snapshot.NetworkService, snapshot.HTTPS))
	return restoreErr
}

func readSystemProxySetting(ctx context.Context, command, service string) (runtime.SystemProxySetting, error) {
	output, err := runCommand(ctx, "/usr/sbin/networksetup", command, service)
	if err != nil {
		return runtime.SystemProxySetting{}, err
	}
	return parseSystemProxySetting(output)
}

func parseSystemProxySetting(output string) (runtime.SystemProxySetting, error) {
	setting := runtime.SystemProxySetting{}
	foundEnabled := false
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Enabled":
			parsed, valid := parseProxyBool(value)
			if !valid {
				return runtime.SystemProxySetting{}, fmt.Errorf("unrecognized system proxy enabled value %q", value)
			}
			setting.Enabled = parsed
			foundEnabled = true
		case "Server":
			setting.Server = value
		case "Port":
			if value == "" {
				continue
			}
			port, err := strconv.Atoi(value)
			if err != nil {
				return runtime.SystemProxySetting{}, fmt.Errorf("invalid system proxy port %q", value)
			}
			setting.Port = port
		case "Authenticated Proxy Enabled":
			parsed, valid := parseProxyBool(value)
			if !valid {
				return runtime.SystemProxySetting{}, fmt.Errorf("unrecognized authenticated proxy value %q", value)
			}
			setting.Authenticated = parsed
		}
	}
	if !foundEnabled {
		return runtime.SystemProxySetting{}, fmt.Errorf("system proxy output did not contain an enabled state")
	}
	return setting, nil
}

func readProxyFeatureEnabled(ctx context.Context, command, service string) (bool, error) {
	output, err := runCommand(ctx, "/usr/sbin/networksetup", command, service)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "enabled" && !strings.Contains(key, "discovery") {
			continue
		}
		if parsed, valid := parseProxyBool(strings.TrimSpace(value)); valid {
			return parsed, nil
		}
	}
	return false, fmt.Errorf("%s output did not contain a recognizable enabled state", command)
}

func parseProxyBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "on", "1", "true":
		return true, true
	case "no", "off", "0", "false":
		return false, true
	default:
		return false, false
	}
}

func setSystemProxyEndpoint(ctx context.Context, command, service, host string, port int) error {
	_, err := runCommand(ctx, "/usr/sbin/networksetup", command, service, host, strconv.Itoa(port), "off")
	return err
}

func verifySystemProxyEndpoint(ctx context.Context, command, service string, port int) error {
	setting, err := readSystemProxySetting(ctx, command, service)
	if err != nil {
		return err
	}
	if !setting.Enabled || setting.Server != localSystemProxyHost || setting.Port != port || setting.Authenticated {
		return fmt.Errorf("network service %q did not apply %s %s:%d", service, command, localSystemProxyHost, port)
	}
	return nil
}

func restoreSystemProxySetting(ctx context.Context, endpointCommand, stateCommand, service string, setting runtime.SystemProxySetting) error {
	if setting.Authenticated {
		return fmt.Errorf("cannot restore authenticated proxy settings for network service %q without credentials", service)
	}
	if setting.Server != "" && setting.Port > 0 {
		if err := setSystemProxyEndpoint(ctx, endpointCommand, service, setting.Server, setting.Port); err != nil {
			return err
		}
	}
	state := "off"
	if setting.Enabled {
		state = "on"
	}
	_, err := runCommand(ctx, "/usr/sbin/networksetup", stateCommand, service, state)
	return err
}
