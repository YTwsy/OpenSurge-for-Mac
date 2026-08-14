package controlapi

import (
	"fmt"
	"net"

	"open-mihomo-gateway/internal/macosnetwork"
)

const minimumSuggestedDHCPHosts = 32

func suggestDHCPRange24(snapshot macosnetwork.Snapshot, protected []string) (string, string, error) {
	gateway := net.ParseIP(snapshot.IPv4).To4()
	maskIP := net.ParseIP(snapshot.SubnetMask).To4()
	if gateway == nil || maskIP == nil {
		return "", "", fmt.Errorf("当前网络没有可用于自动填写的完整 IPv4 与子网掩码")
	}
	ones, bits := net.IPMask(maskIP).Size()
	if bits != 32 || ones != 24 {
		return "", "", fmt.Errorf("当前网络掩码为 %s；v0.1.26 只对 /24 网络自动生成 DHCP 地址池", snapshot.SubnetMask)
	}

	reserved := map[byte]bool{gateway[3]: true}
	for _, value := range append([]string{snapshot.Router}, protected...) {
		ip := net.ParseIP(value).To4()
		if ip != nil && ip[0] == gateway[0] && ip[1] == gateway[1] && ip[2] == gateway[2] {
			reserved[ip[3]] = true
		}
	}

	start, end, ok := longestAvailableHostRun(reserved, 100, 200)
	if !ok || int(end)-int(start)+1 < minimumSuggestedDHCPHosts {
		start, end, ok = longestAvailableHostRun(reserved, 20, 240)
	}
	if !ok || int(end)-int(start)+1 < minimumSuggestedDHCPHosts {
		return "", "", fmt.Errorf("当前 /24 网络没有足够连续的安全地址可自动生成 DHCP 地址池")
	}
	if int(end)-int(start)+1 > 101 {
		end = start + 100
	}
	prefix := fmt.Sprintf("%d.%d.%d", gateway[0], gateway[1], gateway[2])
	return fmt.Sprintf("%s.%d", prefix, start), fmt.Sprintf("%s.%d", prefix, end), nil
}

func longestAvailableHostRun(reserved map[byte]bool, lower, upper byte) (byte, byte, bool) {
	var bestStart, bestEnd, currentStart byte
	bestLength, currentLength := 0, 0
	for host := int(lower); host <= int(upper); host++ {
		if reserved[byte(host)] {
			currentLength = 0
			continue
		}
		if currentLength == 0 {
			currentStart = byte(host)
		}
		currentLength++
		if currentLength > bestLength {
			bestLength = currentLength
			bestStart = currentStart
			bestEnd = byte(host)
		}
	}
	return bestStart, bestEnd, bestLength > 0
}
