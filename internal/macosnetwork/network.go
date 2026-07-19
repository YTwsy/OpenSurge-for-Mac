package macosnetwork

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Snapshot struct {
	NetworkService string   `json:"network_service"`
	Interface      string   `json:"interface"`
	HardwareAddr   string   `json:"hardware_address,omitempty"`
	IPv4           string   `json:"ipv4,omitempty"`
	SubnetMask     string   `json:"subnet_mask,omitempty"`
	Router         string   `json:"router,omitempty"`
	DNS            []string `json:"dns"`
	IPv6Default    bool     `json:"ipv6_default"`
}

type ManualConfig struct {
	NetworkService string   `json:"network_service"`
	Interface      string   `json:"interface"`
	IPv4           string   `json:"ipv4"`
	SubnetMask     string   `json:"subnet_mask"`
	Router         string   `json:"router"`
	DNS            []string `json:"dns"`
}

type InterfaceOption struct {
	Interface      string `json:"interface"`
	NetworkService string `json:"network_service"`
}

var runCommand = run

func Discover(ctx context.Context, networkService, interfaceName string) (Snapshot, error) {
	if strings.TrimSpace(networkService) == "" {
		var err error
		networkService, err = NetworkServiceForInterface(ctx, interfaceName)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if strings.TrimSpace(interfaceName) == "" {
		return Snapshot{}, fmt.Errorf("interface is required")
	}
	actualInterface, err := ServiceInterface(ctx, networkService)
	if err != nil {
		return Snapshot{}, err
	}
	if actualInterface != interfaceName {
		return Snapshot{}, fmt.Errorf("network service %q uses %s, not %s", networkService, actualInterface, interfaceName)
	}
	info, err := runCommand(ctx, "/usr/sbin/networksetup", "-getinfo", networkService)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := parseNetworkInfo(info)
	snapshot.NetworkService = networkService
	snapshot.Interface = interfaceName
	if dns, err := runCommand(ctx, "/usr/sbin/networksetup", "-getdnsservers", networkService); err == nil {
		snapshot.DNS = parseDNS(dns)
	}
	if iface, err := net.InterfaceByName(interfaceName); err == nil {
		snapshot.HardwareAddr = iface.HardwareAddr.String()
	}
	if routes, err := runCommand(ctx, "/usr/sbin/netstat", "-rn", "-f", "inet6"); err == nil {
		snapshot.IPv6Default = hasIPv6DefaultRoute(routes, interfaceName)
	}
	if snapshot.IPv4 == "" || snapshot.SubnetMask == "" || snapshot.Router == "" {
		return Snapshot{}, fmt.Errorf("network service %q does not expose a complete IPv4 configuration", networkService)
	}
	return snapshot, nil
}

func SetManual(ctx context.Context, cfg ManualConfig) error {
	if err := ValidateManual(cfg); err != nil {
		return err
	}
	if _, err := runCommand(ctx, "/usr/sbin/networksetup", "-setmanual", cfg.NetworkService, cfg.IPv4, cfg.SubnetMask, cfg.Router); err != nil {
		return err
	}
	dns := cfg.DNS
	if len(dns) == 0 {
		dns = []string{"Empty"}
	}
	_, err := runCommand(ctx, "/usr/sbin/networksetup", append([]string{"-setdnsservers", cfg.NetworkService}, dns...)...)
	return err
}

func ValidateManual(cfg ManualConfig) error {
	ip := net.ParseIP(cfg.IPv4).To4()
	maskIP := net.ParseIP(cfg.SubnetMask).To4()
	router := net.ParseIP(cfg.Router).To4()
	if ip == nil || maskIP == nil || router == nil {
		return fmt.Errorf("manual network configuration requires valid IPv4, subnet mask, and router")
	}
	mask := net.IPMask(maskIP)
	ones, bits := mask.Size()
	if bits != 32 || ones <= 0 || ones >= 32 {
		return fmt.Errorf("manual network configuration requires a contiguous unicast subnet mask")
	}
	if !ip.Mask(mask).Equal(router.Mask(mask)) {
		return fmt.Errorf("manual IPv4 and router must share a subnet")
	}
	if ip.Equal(router) {
		return fmt.Errorf("manual IPv4 must differ from router")
	}
	if strings.TrimSpace(cfg.NetworkService) == "" || strings.TrimSpace(cfg.Interface) == "" {
		return fmt.Errorf("network service and interface are required")
	}
	for _, server := range cfg.DNS {
		if net.ParseIP(server) == nil {
			return fmt.Errorf("invalid DNS server %q", server)
		}
	}
	return nil
}

func ServiceInterface(ctx context.Context, networkService string) (string, error) {
	if strings.TrimSpace(networkService) == "" {
		return "", fmt.Errorf("network service is required")
	}
	output, err := runCommand(ctx, "/usr/sbin/networksetup", "-listnetworkserviceorder")
	if err != nil {
		return "", err
	}
	return parseServiceInterface(output, networkService)
}

func NetworkServiceForInterface(ctx context.Context, interfaceName string) (string, error) {
	if strings.TrimSpace(interfaceName) == "" {
		return "", fmt.Errorf("interface is required")
	}
	output, err := runCommand(ctx, "/usr/sbin/networksetup", "-listnetworkserviceorder")
	if err != nil {
		return "", err
	}
	services := parseServiceOrder(output)
	for service, device := range services {
		if device == interfaceName {
			return service, nil
		}
	}
	return "", fmt.Errorf("no network service uses interface %q", interfaceName)
}

func ListInterfaces(ctx context.Context) ([]InterfaceOption, error) {
	output, err := runCommand(ctx, "/usr/sbin/networksetup", "-listnetworkserviceorder")
	if err != nil {
		return nil, err
	}
	return interfaceOptions(parseServiceOrder(output)), nil
}

func interfaceOptions(services map[string]string) []InterfaceOption {
	options := make([]InterfaceOption, 0, len(services))
	for service, device := range services {
		if strings.TrimSpace(service) == "" || strings.TrimSpace(device) == "" {
			continue
		}
		options = append(options, InterfaceOption{Interface: device, NetworkService: service})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Interface == options[j].Interface {
			return options[i].NetworkService < options[j].NetworkService
		}
		return options[i].Interface < options[j].Interface
	})
	return options
}

func parseServiceInterface(output, networkService string) (string, error) {
	if device := parseServiceOrder(output)[networkService]; device != "" {
		return device, nil
	}
	return "", fmt.Errorf("network service %q was not found", networkService)
}

func parseServiceOrder(output string) map[string]string {
	result := map[string]string{}
	lines := strings.Split(output, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "(") || !strings.Contains(trimmed, ") ") {
			continue
		}
		_, service, _ := strings.Cut(trimmed, ") ")
		if index+1 >= len(lines) {
			continue
		}
		detail := strings.TrimSpace(lines[index+1])
		marker := "Device: "
		position := strings.Index(detail, marker)
		if position < 0 {
			break
		}
		device := strings.TrimSuffix(strings.TrimSpace(detail[position+len(marker):]), ")")
		if device != "" {
			result[service] = device
		}
	}
	return result
}

func SetDHCP(ctx context.Context, networkService string) error {
	if strings.TrimSpace(networkService) == "" {
		return fmt.Errorf("network service is required")
	}
	if _, err := runCommand(ctx, "/usr/sbin/networksetup", "-setdhcp", networkService); err != nil {
		return err
	}
	_, err := runCommand(ctx, "/usr/sbin/networksetup", "-setdnsservers", networkService, "Empty")
	return err
}

func PingRouter(ctx context.Context, router string) error {
	if net.ParseIP(router).To4() == nil {
		return fmt.Errorf("router must be IPv4")
	}
	_, err := runCommand(ctx, "/sbin/ping", "-c", "1", "-W", "1000", router)
	return err
}

func parseNetworkInfo(output string) Snapshot {
	result := Snapshot{DNS: []string{}}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "IP address":
			result.IPv4 = strings.TrimSpace(value)
		case "Subnet mask":
			result.SubnetMask = strings.TrimSpace(value)
		case "Router":
			result.Router = strings.TrimSpace(value)
		}
	}
	return result
}

func parseDNS(output string) []string {
	if strings.Contains(output, "There aren't any DNS Servers") {
		return []string{}
	}
	var result []string
	for _, line := range strings.Split(output, "\n") {
		value := strings.TrimSpace(line)
		if net.ParseIP(value) != nil {
			result = append(result, value)
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

func hasIPv6DefaultRoute(output, interfaceName string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && (fields[0] == "default" || fields[0] == "::/0") && fields[len(fields)-1] == interfaceName {
			return true
		}
	}
	return false
}

func run(ctx context.Context, binary string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, binary, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", binary, strings.Join(args, " "), err, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}
