package macosipv6

import (
	"net"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/process"
)

var (
	commandOutput   = process.Output
	interfaceByName = net.InterfaceByName
	interfaceAddrs  = func(iface *net.Interface) ([]net.Addr, error) { return iface.Addrs() }
)

type Manager struct {
	cfg config.Config
}

func New(cfg config.Config) Manager {
	return Manager{cfg: cfg}
}

func (m Manager) NativeAvailable() (bool, error) {
	iface, err := interfaceByName(m.cfg.Gateway.UpstreamInterface)
	if err != nil {
		return false, err
	}
	addrs, err := interfaceAddrs(iface)
	if err != nil {
		return false, err
	}
	hasUsableAddress := false
	for _, addr := range addrs {
		ip := addressIP(addr)
		if ip != nil && ip.To4() == nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() {
			hasUsableAddress = true
			break
		}
	}
	if !hasUsableAddress {
		return false, nil
	}
	output, err := commandOutput("/usr/sbin/netstat", "-rn", "-f", "inet6")
	if err != nil {
		return false, err
	}
	return hasDefaultRouteOnInterface(string(output), m.cfg.Gateway.UpstreamInterface), nil
}

func (m Manager) TUNEffective() bool {
	if m.cfg.Transparent.TUNIPv6 == config.TUNIPv6Off {
		return false
	}
	address, _, err := net.ParseCIDR(config.MihomoTUNIPv6)
	if err != nil {
		return false
	}
	present, err := interfaceHasAddress(m.cfg.Transparent.TUNDevice, address.String())
	return err == nil && present
}

func interfaceHasAddress(interfaceName, target string) (bool, error) {
	iface, err := interfaceByName(interfaceName)
	if err != nil {
		return false, err
	}
	addrs, err := interfaceAddrs(iface)
	if err != nil {
		return false, err
	}
	targetIP := net.ParseIP(target)
	for _, addr := range addrs {
		if ip := addressIP(addr); ip != nil && ip.Equal(targetIP) {
			return true, nil
		}
	}
	return false, nil
}

func addressIP(addr net.Addr) net.IP {
	switch value := addr.(type) {
	case *net.IPNet:
		return value.IP
	case *net.IPAddr:
		return value.IP
	default:
		host, _, err := net.SplitHostPort(addr.String())
		if err == nil {
			return net.ParseIP(host)
		}
		return net.ParseIP(strings.Split(addr.String(), "/")[0])
	}
}

func hasDefaultRouteOnInterface(output, interfaceName string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if fields[0] != "default" && fields[0] != "::/0" {
			continue
		}
		if fields[len(fields)-1] == interfaceName {
			return true
		}
	}
	return false
}
