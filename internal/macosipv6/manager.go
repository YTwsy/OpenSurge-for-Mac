package macosipv6

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/process"
)

type Manager struct {
	cfg        config.Config
	runCommand func(context.Context, string, ...string) error
	output     func(string, ...string) ([]byte, error)
	byName     func(string) (*net.Interface, error)
	addrs      func(*net.Interface) ([]net.Addr, error)
	withdraw   func(string, net.HardwareAddr, net.IP, net.IP) error
}

func New(cfg config.Config) Manager {
	return Manager{
		cfg: cfg, runCommand: run, output: process.Output, byName: net.InterfaceByName,
		addrs:    func(iface *net.Interface) ([]net.Addr, error) { return iface.Addrs() },
		withdraw: sendRouterWithdrawal,
	}
}

func (m Manager) NativeAvailable() (bool, error) {
	byName := m.byName
	if byName == nil {
		byName = net.InterfaceByName
	}
	iface, err := byName(m.cfg.Gateway.UpstreamInterface)
	if err != nil {
		return false, err
	}
	readAddrs := m.addrs
	if readAddrs == nil {
		readAddrs = func(iface *net.Interface) ([]net.Addr, error) { return iface.Addrs() }
	}
	addrs, err := readAddrs(iface)
	if err != nil {
		return false, err
	}
	for _, addr := range addrs {
		ipText := addr.String()
		if before, _, ok := strings.Cut(ipText, "/"); ok {
			ipText = before
		}
		ip := net.ParseIP(ipText)
		// net.IP.IsGlobalUnicast intentionally includes IPv6 unique-local
		// addresses. A ULA plus a router-advertised default route does not prove
		// that DIRECT can reach the public IPv6 Internet, so auto takeover must
		// require a non-private global-unicast address on the upstream interface.
		if ip != nil && ip.To4() == nil && ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() && !ip.IsPrivate() {
			output := m.output
			if output == nil {
				output = process.Output
			}
			routes, err := output("/usr/sbin/netstat", "-rn", "-f", "inet6")
			if err != nil {
				return false, err
			}
			return hasDefaultRouteOnInterface(string(routes), iface.Name), nil
		}
	}
	return false, nil
}

func hasDefaultRouteOnInterface(output, interfaceName string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || (fields[0] != "default" && fields[0] != "::/0") {
			continue
		}
		last := len(fields) - 1
		if fields[last] == interfaceName || (last > 0 && fields[last-1] == interfaceName) {
			return true
		}
	}
	return false
}

func (m Manager) CheckGatewayAvailable() error {
	present, err := m.gatewayAddressPresent()
	if err != nil {
		return err
	}
	if present {
		return fmt.Errorf("downstream IPv6 gateway %s already exists on %s without OpenSurge runtime ownership", config.DownstreamIPv6Gateway, m.cfg.Gateway.Interface)
	}
	return nil
}

func (m Manager) gatewayAddressPresent() (bool, error) {
	byName := m.byName
	if byName == nil {
		byName = net.InterfaceByName
	}
	iface, err := byName(m.cfg.Gateway.Interface)
	if err != nil {
		return false, err
	}
	readAddrs := m.addrs
	if readAddrs == nil {
		readAddrs = func(iface *net.Interface) ([]net.Addr, error) { return iface.Addrs() }
	}
	addrs, err := readAddrs(iface)
	if err != nil {
		return false, err
	}
	target := net.ParseIP(config.DownstreamIPv6Gateway)
	for _, addr := range addrs {
		address := addr.String()
		if before, _, ok := strings.Cut(address, "/"); ok {
			address = before
		}
		if before, _, ok := strings.Cut(address, "%"); ok {
			address = before
		}
		if ip := net.ParseIP(address); ip != nil && ip.Equal(target) {
			return true, nil
		}
	}
	return false, nil
}

func (m Manager) AddGateway(ctx context.Context) error {
	if err := m.command(ctx, "/sbin/ifconfig", m.cfg.Gateway.Interface, "inet6", config.DownstreamIPv6Gateway, "prefixlen", "64", "alias"); err != nil {
		return err
	}
	if err := m.waitGatewayReady(ctx); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cleanupErr := m.RemoveGateway(cleanupCtx)
		return errors.Join(err, cleanupErr)
	}
	return nil
}

func (m Manager) waitGatewayReady(ctx context.Context) error {
	output := m.output
	if output == nil {
		output = process.Output
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := output("/sbin/ifconfig", m.cfg.Gateway.Interface, "inet6")
		if err != nil {
			return fmt.Errorf("inspect downstream IPv6 gateway readiness: %w", err)
		}
		ready, duplicated := gatewayAddressState(string(data), config.DownstreamIPv6Gateway)
		if duplicated {
			return fmt.Errorf("downstream IPv6 gateway %s is duplicated on %s", config.DownstreamIPv6Gateway, m.cfg.Gateway.Interface)
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("downstream IPv6 gateway %s remained tentative on %s after 5s", config.DownstreamIPv6Gateway, m.cfg.Gateway.Interface)
		case <-ticker.C:
		}
	}
}

func gatewayAddressState(output, target string) (ready, duplicated bool) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "inet6" || strings.SplitN(fields[1], "%", 2)[0] != target {
			continue
		}
		for _, field := range fields[2:] {
			switch field {
			case "duplicated":
				return false, true
			case "tentative":
				return false, false
			}
		}
		return true, false
	}
	return false, false
}

func (m Manager) RemoveGateway(ctx context.Context) error {
	present, err := m.gatewayAddressPresent()
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	return m.command(ctx, "/sbin/ifconfig", m.cfg.Gateway.Interface, "inet6", config.DownstreamIPv6Gateway, "-alias")
}

func (m Manager) Withdraw(ctx context.Context) error {
	byName := m.byName
	if byName == nil {
		byName = net.InterfaceByName
	}
	iface, err := byName(m.cfg.Gateway.Interface)
	if err != nil {
		return err
	}
	dns, err := m.linkLocalAddress(iface)
	if err != nil {
		return err
	}
	prefixIP, _, _ := net.ParseCIDR(config.DownstreamIPv6Prefix)
	if err := m.withdraw(iface.Name, iface.HardwareAddr, prefixIP, dns); err != nil {
		return fmt.Errorf("send IPv6 router withdrawal: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return nil
}

func (m Manager) linkLocalAddress(iface *net.Interface) (net.IP, error) {
	readAddrs := m.addrs
	if readAddrs == nil {
		readAddrs = func(iface *net.Interface) ([]net.Addr, error) { return iface.Addrs() }
	}
	addrs, err := readAddrs(iface)
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		address := addr.String()
		if before, _, ok := strings.Cut(address, "/"); ok {
			address = before
		}
		if before, _, ok := strings.Cut(address, "%"); ok {
			address = before
		}
		if ip := net.ParseIP(address); ip != nil && ip.To4() == nil && ip.IsLinkLocalUnicast() {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("downstream interface %s has no IPv6 link-local address for RDNSS withdrawal", iface.Name)
}

func (m Manager) command(ctx context.Context, name string, args ...string) error {
	runner := m.runCommand
	if runner == nil {
		runner = run
	}
	return runner(ctx, name, args...)
}

func run(ctx context.Context, name string, args ...string) error {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
