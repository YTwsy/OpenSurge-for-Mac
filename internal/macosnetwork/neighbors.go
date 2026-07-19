package macosnetwork

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// Neighbor is a currently cached IPv4-to-MAC mapping on a macOS interface.
// The ARP cache is observation evidence only; it is not an ownership or
// authentication guarantee.
type Neighbor struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Interface string `json:"interface"`
}

func DiscoverNeighbors(ctx context.Context, interfaceName string) ([]Neighbor, error) {
	interfaceName = strings.TrimSpace(interfaceName)
	if interfaceName == "" {
		return nil, fmt.Errorf("interface is required")
	}
	output, err := runCommand(ctx, "/usr/sbin/arp", "-an", "-i", interfaceName)
	if err != nil {
		return nil, err
	}
	return parseNeighbors(output, interfaceName), nil
}

func parseNeighbors(output, interfaceName string) []Neighbor {
	neighbors := []Neighbor{}
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[2] != "at" || fields[4] != "on" || fields[5] != interfaceName {
			continue
		}
		ipText := strings.Trim(fields[1], "()")
		ip := net.ParseIP(ipText).To4()
		mac, err := net.ParseMAC(fields[3])
		if ip == nil || err != nil || seen[ip.String()] {
			continue
		}
		seen[ip.String()] = true
		neighbors = append(neighbors, Neighbor{IP: ip.String(), MAC: strings.ToLower(mac.String()), Interface: interfaceName})
	}
	return neighbors
}
