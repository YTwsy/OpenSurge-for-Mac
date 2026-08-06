package ipv6packet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
)

const (
	ethernetHeaderSize = 14
	etherTypeIPv6      = 0x86dd
)

var downstreamIPv6Prefix = netip.MustParsePrefix(config.DownstreamIPv6Prefix)

type BrokerConfig struct {
	Interface string
	Socket    string
	ReadyFile string
	MTU       int
}

type frameDevice interface {
	ReadFrame() ([]byte, error)
	WriteFrame([]byte) error
	Close() error
}

var openPacketDevice = openBPFPacketDevice

type neighborTable struct {
	mu       sync.RWMutex
	byIPv6   map[netip.Addr]net.HardwareAddr
	conflict uint64
}

func newNeighborTable() *neighborTable {
	return &neighborTable{byIPv6: map[netip.Addr]net.HardwareAddr{}}
}

func (t *neighborTable) Observe(ip netip.Addr, mac net.HardwareAddr) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if current, ok := t.byIPv6[ip]; ok && current.String() != mac.String() {
		t.conflict++
		return false
	}
	t.byIPv6[ip] = append(net.HardwareAddr(nil), mac...)
	return true
}

func (t *neighborTable) Lookup(ip netip.Addr) (net.HardwareAddr, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	mac, ok := t.byIPv6[ip]
	return append(net.HardwareAddr(nil), mac...), ok
}

func RunBroker(ctx context.Context, cfg BrokerConfig) error {
	if cfg.Interface == "" || cfg.Socket == "" || cfg.ReadyFile == "" {
		return fmt.Errorf("interface, socket, and ready-file are required")
	}
	if cfg.MTU < 1280 || cfg.MTU > 9000 {
		return fmt.Errorf("mtu must be between 1280 and 9000")
	}
	iface, err := net.InterfaceByName(cfg.Interface)
	if err != nil {
		return fmt.Errorf("resolve downstream interface %s: %w", cfg.Interface, err)
	}
	if len(iface.HardwareAddr) != 6 {
		return fmt.Errorf("downstream interface %s is not Ethernet", cfg.Interface)
	}
	device, err := openPacketDevice(cfg.Interface, cfg.MTU)
	if err != nil {
		return err
	}
	defer device.Close()

	localSocket := cfg.Socket + ".broker"
	if err := removeOwnedSocket(localSocket); err != nil {
		return err
	}
	localAddr := &net.UnixAddr{Name: localSocket, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", localAddr)
	if err != nil {
		return fmt.Errorf("listen on IPv6 packet broker socket: %w", err)
	}
	defer func() {
		_ = conn.Close()
		_ = os.Remove(localSocket)
		_ = os.Remove(cfg.ReadyFile)
	}()
	if err := os.Chmod(localSocket, 0o600); err != nil {
		return fmt.Errorf("secure IPv6 packet broker socket: %w", err)
	}
	serverAddr := &net.UnixAddr{Name: cfg.Socket, Net: "unixgram"}
	if err := waitForListener(ctx, conn, serverAddr); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.ReadyFile, []byte("ready\n"), 0o600); err != nil {
		return fmt.Errorf("write IPv6 packet broker readiness: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	neighbors := newNeighborTable()
	go func() { errCh <- readOutbound(runCtx, conn, device, iface.HardwareAddr, neighbors, cfg.MTU) }()
	go func() { errCh <- readInbound(runCtx, conn, serverAddr, device, iface.HardwareAddr, neighbors, cfg.MTU) }()
	select {
	case <-ctx.Done():
		_ = conn.Close()
		_ = device.Close()
		return nil
	case err := <-errCh:
		cancel()
		_ = conn.Close()
		_ = device.Close()
		if err == nil || errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) {
			return nil
		}
		return err
	}
}

func waitForListener(ctx context.Context, conn *net.UnixConn, server *net.UnixAddr) error {
	hello, _ := EncodeMessage(MessageHello, nil, nil)
	deadline := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	for {
		if _, err := conn.WriteToUnix(hello, server); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
		}
	}
}

func readInbound(ctx context.Context, conn *net.UnixConn, server *net.UnixAddr, device frameDevice, gatewayMAC net.HardwareAddr, neighbors *neighborTable, mtu int) error {
	accepted := uint64(0)
	seen := uint64(0)
	diagnosedCandidates := uint64(0)
	for {
		frame, err := device.ReadFrame()
		if err != nil {
			return fmt.Errorf("read downstream IPv6 frame: %w", err)
		}
		seen++
		if seen == 1 && len(frame) >= ethernetHeaderSize {
			log.Printf("OpenSurge IPv6 BPF frame source-mac=%s destination-mac=%s bytes=%d", net.HardwareAddr(frame[6:12]), net.HardwareAddr(frame[:6]), len(frame))
		}
		packet, sourceIP, sourceMAC, ok := parseInboundFrame(frame, gatewayMAC, mtu)
		if diagnosedCandidates < 5 {
			if candidate, candidateSource, candidateDestination := diagnoseInboundCandidate(frame, gatewayMAC); candidate {
				diagnosedCandidates++
				log.Printf("OpenSurge IPv6 BPF ingress candidate=%d source=%s destination=%s source-mac=%s destination-mac=%s gateway-mac-match=%t accepted=%t bytes=%d", diagnosedCandidates, candidateSource, candidateDestination, net.HardwareAddr(frame[6:12]), net.HardwareAddr(frame[:6]), equalMAC(frame[:6], gatewayMAC), ok, len(frame))
			}
		}
		if !ok || !neighbors.Observe(sourceIP, sourceMAC) {
			continue
		}
		accepted++
		if accepted == 1 || accepted%100 == 0 {
			destination, _ := ipv6Destination(packet)
			log.Printf("OpenSurge IPv6 packet ingress accepted=%d source=%s mac=%s destination=%s bytes=%d", accepted, sourceIP, sourceMAC, destination, len(packet))
		}
		message, err := EncodeMessage(MessageInbound, sourceMAC, packet)
		if err != nil {
			continue
		}
		if _, err := conn.WriteToUnix(message, server); err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				continue
			}
		}
	}
}

func diagnoseInboundCandidate(frame []byte, gatewayMAC net.HardwareAddr) (bool, netip.Addr, netip.Addr) {
	if len(frame) < ethernetHeaderSize+40 || frame[12] != 0x86 || frame[13] != 0xdd || equalMAC(frame[6:12], gatewayMAC) {
		return false, netip.Addr{}, netip.Addr{}
	}
	packet := frame[ethernetHeaderSize:]
	if packet[0]>>4 != 6 {
		return false, netip.Addr{}, netip.Addr{}
	}
	source := netip.AddrFrom16([16]byte(packet[8:24]))
	destination := netip.AddrFrom16([16]byte(packet[24:40]))
	if !source.IsGlobalUnicast() || !destination.IsGlobalUnicast() || downstreamIPv6Prefix.Contains(destination) {
		return false, netip.Addr{}, netip.Addr{}
	}
	return true, source, destination
}

func readOutbound(ctx context.Context, conn *net.UnixConn, device frameDevice, gatewayMAC net.HardwareAddr, neighbors *neighborTable, mtu int) error {
	buffer := make([]byte, mtu+256)
	written := uint64(0)
	for {
		n, _, err := conn.ReadFromUnix(buffer)
		if err != nil {
			return err
		}
		message, err := DecodeMessage(buffer[:n])
		if err != nil || message.Type != MessageOutbound {
			continue
		}
		destination, ok := ipv6Destination(message.Packet)
		if !ok {
			continue
		}
		destinationMAC, ok := neighbors.Lookup(destination)
		if !ok {
			continue
		}
		frame := make([]byte, ethernetHeaderSize+len(message.Packet))
		copy(frame[:6], destinationMAC)
		copy(frame[6:12], gatewayMAC)
		frame[12], frame[13] = byte(etherTypeIPv6>>8), byte(etherTypeIPv6&0xff)
		copy(frame[ethernetHeaderSize:], message.Packet)
		if err := device.WriteFrame(frame); err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("write downstream IPv6 frame: %w", err)
			}
		}
		written++
		if written == 1 || written%100 == 0 {
			log.Printf("OpenSurge IPv6 packet egress written=%d destination=%s mac=%s bytes=%d", written, destination, destinationMAC, len(message.Packet))
		}
	}
}

func parseInboundFrame(frame []byte, gatewayMAC net.HardwareAddr, mtu int) ([]byte, netip.Addr, net.HardwareAddr, bool) {
	if len(frame) < ethernetHeaderSize+40 || len(frame) > ethernetHeaderSize+mtu || frame[12] != 0x86 || frame[13] != 0xdd {
		return nil, netip.Addr{}, nil, false
	}
	if !equalMAC(frame[:6], gatewayMAC) || equalMAC(frame[6:12], gatewayMAC) {
		return nil, netip.Addr{}, nil, false
	}
	packet := frame[ethernetHeaderSize:]
	if packet[0]>>4 != 6 {
		return nil, netip.Addr{}, nil, false
	}
	source := netip.AddrFrom16([16]byte(packet[8:24]))
	destination := netip.AddrFrom16([16]byte(packet[24:40]))
	// The packet path owns exactly one downstream /64. Do not let a client use
	// an arbitrary spoofed global/ULA source to populate the identity and
	// return-neighbour tables; traffic outside the advertised prefix is not an
	// OpenSurge downstream flow.
	if !downstreamIPv6Prefix.Contains(source) || !destination.IsGlobalUnicast() {
		return nil, netip.Addr{}, nil, false
	}
	// Packets for the on-link downstream /64 belong to the host or another
	// client. Only off-link IPv6 routed to the Mac enters the proxy stack.
	if downstreamIPv6Prefix.Contains(destination) {
		return nil, netip.Addr{}, nil, false
	}
	return append([]byte(nil), packet...), source, append(net.HardwareAddr(nil), frame[6:12]...), true
}

func ipv6Destination(packet []byte) (netip.Addr, bool) {
	if len(packet) < 40 || packet[0]>>4 != 6 {
		return netip.Addr{}, false
	}
	return netip.AddrFrom16([16]byte(packet[24:40])), true
}

func equalMAC(a []byte, b net.HardwareAddr) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func removeOwnedSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to replace non-socket path %s", path)
	}
	return os.Remove(path)
}

func EnsureSocketParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
