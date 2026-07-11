package macosnetwork

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"syscall"
	"time"
)

const ipBoundIf = 25 // IP_BOUND_IF on Darwin.

func ProbeDHCPServers(ctx context.Context, interfaceName string, timeout time.Duration) ([]string, error) {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, err
	}
	if len(iface.HardwareAddr) < 6 {
		return nil, fmt.Errorf("interface %s has no Ethernet hardware address", interfaceName)
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 68})
	if err != nil {
		return nil, fmt.Errorf("listen for DHCP offers: %w", err)
	}
	defer conn.Close()
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		if e := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1); e != nil {
			socketErr = e
			return
		}
		if e := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipBoundIf, iface.Index); e != nil {
			socketErr = e
		}
	}); err != nil {
		return nil, err
	}
	if socketErr != nil {
		return nil, socketErr
	}
	packet, xid, err := dhcpDiscover(iface.HardwareAddr)
	if err != nil {
		return nil, err
	}
	if _, err := conn.WriteToUDP(packet, &net.UDPAddr{IP: net.IPv4bcast, Port: 67}); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	servers := map[string]bool{}
	buffer := make([]byte, 1500)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(minTime(deadline, time.Now().Add(250*time.Millisecond)))
		n, _, err := conn.ReadFromUDP(buffer)
		if timeoutErr, ok := err.(net.Error); ok && timeoutErr.Timeout() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		if server, ok := parseDHCPOffer(buffer[:n], xid); ok {
			servers[server] = true
		}
	}
	result := make([]string, 0, len(servers))
	for server := range servers {
		result = append(result, server)
	}
	sort.Strings(result)
	return result, nil
}

func dhcpDiscover(mac net.HardwareAddr) ([]byte, uint32, error) {
	var xidBytes [4]byte
	if _, err := rand.Read(xidBytes[:]); err != nil {
		return nil, 0, err
	}
	xid := binary.BigEndian.Uint32(xidBytes[:])
	packet := make([]byte, 244)
	packet[0], packet[1], packet[2] = 1, 1, 6
	binary.BigEndian.PutUint32(packet[4:8], xid)
	binary.BigEndian.PutUint16(packet[10:12], 0x8000)
	copy(packet[28:34], mac[:6])
	copy(packet[236:240], []byte{99, 130, 83, 99})
	packet = append(packet, 53, 1, 1, 55, 4, 1, 3, 6, 54, 255)
	return packet, xid, nil
}

func parseDHCPOffer(packet []byte, xid uint32) (string, bool) {
	if len(packet) < 244 || packet[0] != 2 || binary.BigEndian.Uint32(packet[4:8]) != xid || string(packet[236:240]) != string([]byte{99, 130, 83, 99}) {
		return "", false
	}
	messageType := byte(0)
	server := net.IP(nil)
	for i := 240; i < len(packet); {
		code := packet[i]
		i++
		if code == 255 {
			break
		}
		if code == 0 {
			continue
		}
		if i >= len(packet) {
			return "", false
		}
		length := int(packet[i])
		i++
		if i+length > len(packet) {
			return "", false
		}
		value := packet[i : i+length]
		i += length
		switch code {
		case 53:
			if length == 1 {
				messageType = value[0]
			}
		case 54:
			if length == 4 {
				server = net.IPv4(value[0], value[1], value[2], value[3])
			}
		}
	}
	if messageType != 2 || server == nil {
		return "", false
	}
	return server.String(), true
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
