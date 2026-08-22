package controlapi

import (
	"fmt"
	"net"

	"open-mihomo-gateway/internal/lan"
	"open-mihomo-gateway/internal/macosnetwork"
)

const (
	minimumSuggestedDHCPHosts = 32
	maximumSuggestedDHCPHosts = 101
)

// snapshotPrefixLen reads the real prefix length of the network the Mac is on.
// The gateway derives NAT and TUN route exclusion from it, so guessing /24 would
// either leak downstream traffic or exclude addresses that are not local.
func snapshotPrefixLen(snapshot macosnetwork.Snapshot) (int, error) {
	maskIP := net.ParseIP(snapshot.SubnetMask).To4()
	if maskIP == nil {
		return 0, fmt.Errorf("当前网络没有可用于自动填写的子网掩码")
	}
	ones, bits := net.IPMask(maskIP).Size()
	if bits != 32 || ones == 0 {
		return 0, fmt.Errorf("当前网络掩码 %s 不是连续的 IPv4 子网掩码", snapshot.SubnetMask)
	}
	if !lan.ValidPrefixLen(ones) {
		return 0, fmt.Errorf("当前网络掩码 %s 对应 /%d，超出网关支持的 /%d–/%d 范围", snapshot.SubnetMask, ones, lan.MinPrefixLen, lan.MaxPrefixLen)
	}
	return ones, nil
}

// suggestDHCPRange picks a pool inside the network the Mac is actually attached
// to. It prefers the middle of the subnet, where consumer routers are least
// likely to have handed out static addresses.
func suggestDHCPRange(snapshot macosnetwork.Snapshot, protected []string) (string, string, error) {
	prefixLen, err := snapshotPrefixLen(snapshot)
	if err != nil {
		return "", "", err
	}
	scope, err := lan.NewScope(snapshot.IPv4, prefixLen)
	if err != nil {
		return "", "", fmt.Errorf("当前网络没有可用于自动填写的完整 IPv4 与子网掩码")
	}

	reserved := map[int]bool{}
	for _, value := range append([]string{snapshot.IPv4, snapshot.Router}, protected...) {
		if offset, ok := scope.Offset(net.ParseIP(value)); ok {
			reserved[offset] = true
		}
	}

	hosts := scope.HostCount()
	minimumHosts := minimumSuggestedDHCPHosts
	if hosts/2 < minimumHosts {
		minimumHosts = hosts / 2
	}
	// The last window is the whole subnet: only subnets too small for the
	// preferred windows reach it, and a cramped pool still beats a blocker.
	windows := [][2]int{{100, 200}, {20, 240}, {1, hosts}}
	for _, window := range windows {
		start, end, ok := longestAvailableHostRun(reserved, clampHost(window[0], hosts), clampHost(window[1], hosts))
		if !ok || end-start+1 < minimumHosts {
			continue
		}
		if end-start+1 > maximumSuggestedDHCPHosts {
			end = start + maximumSuggestedDHCPHosts - 1
		}
		return scope.HostAt(start).String(), scope.HostAt(end).String(), nil
	}
	return "", "", fmt.Errorf("当前 %s 网络没有足够连续的安全地址可自动生成 DHCP 地址池", scope)
}

func clampHost(offset, hosts int) int {
	if offset < 1 {
		return 1
	}
	if offset > hosts {
		return hosts
	}
	return offset
}

func longestAvailableHostRun(reserved map[int]bool, lower, upper int) (int, int, bool) {
	bestStart, bestEnd, currentStart := 0, 0, 0
	bestLength, currentLength := 0, 0
	for host := lower; host <= upper; host++ {
		if reserved[host] {
			currentLength = 0
			continue
		}
		if currentLength == 0 {
			currentStart = host
		}
		currentLength++
		if currentLength > bestLength {
			bestLength = currentLength
			bestStart = currentStart
			bestEnd = host
		}
	}
	return bestStart, bestEnd, bestLength > 0
}
