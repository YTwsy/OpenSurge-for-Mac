package macosipv6

import (
	"encoding/binary"
	"fmt"
	"net"
)

func routerWithdrawalPayload(mac net.HardwareAddr, prefix, dns net.IP) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("router advertisement requires a 6-byte Ethernet MAC")
	}
	prefix = prefix.To16()
	dns = dns.To16()
	if prefix == nil || dns == nil {
		return nil, fmt.Errorf("router advertisement requires IPv6 prefix and DNS addresses")
	}
	payload := make([]byte, 16+8+32+24)
	payload[0] = 134 // Router Advertisement
	payload[4] = 64  // Cur Hop Limit
	// Router lifetime, reachable time, and retrans timer remain zero.
	payload[16] = 1 // Source Link-Layer Address
	payload[17] = 1
	copy(payload[18:24], mac)
	payload[24] = 3 // Prefix Information
	payload[25] = 4
	payload[26] = 64
	payload[27] = 0xc0 // on-link + autonomous
	// Valid and preferred lifetimes are zero during withdrawal.
	copy(payload[40:56], prefix)
	payload[56] = 25 // Recursive DNS Server
	payload[57] = 3
	binary.BigEndian.PutUint32(payload[60:64], 0)
	copy(payload[64:80], dns)
	return payload, nil
}
