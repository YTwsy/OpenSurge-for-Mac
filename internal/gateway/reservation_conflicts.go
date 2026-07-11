package gateway

import (
	"net"
	"os/exec"
	"strings"

	"open-mihomo-gateway/internal/config"
)

// checkReservationConflicts is intentionally limited to same-WiFi DHCP
// takeover. Isolated lab LANs have no pre-existing neighbour population, while
// a real shared L2 needs a final live check in addition to declarative
// protected_ipv4 validation.
func (m Manager) checkReservationConflicts(deps gatewayDeps) error {
	if m.cfg.Gateway.Mode != config.GatewayModeSameWiFiDHCP || m.cfg.DevicePolicy.Bundle == nil {
		return nil
	}
	probe := deps.probeReservationIP
	if probe == nil {
		probe = probeReservationIPConflict
	}
	for _, reservation := range m.cfg.DevicePolicy.Bundle.Compiled.Reservations {
		if err := probe(reservation.IPv4, reservation.MAC); err != nil {
			return err
		}
	}
	return nil
}

// probeReservationIPConflict warms the ARP cache with one ICMP probe, then
// treats a different observed MAC as a hard conflict. No reply is not a proof
// of vacancy (sleeping hosts and firewalls exist), so it remains non-fatal.
func probeReservationIPConflict(ip string, expectedMAC string) error {
	_ = exec.Command("/sbin/ping", "-c", "1", "-W", "1000", ip).Run()
	output, err := exec.Command("/usr/sbin/arp", "-n", ip).Output()
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(output))
	for index, field := range fields {
		if field != "at" || index+1 >= len(fields) {
			continue
		}
		observed, err := net.ParseMAC(fields[index+1])
		if err != nil || len(observed) != 6 {
			return nil
		}
		expected, err := net.ParseMAC(expectedMAC)
		if err != nil || len(expected) != 6 {
			return nil
		}
		if !strings.EqualFold(observed.String(), expected.String()) {
			return &reservationConflictError{ip: ip, observedMAC: observed.String(), expectedMAC: expected.String()}
		}
		return nil
	}
	return nil
}

type reservationConflictError struct {
	ip          string
	observedMAC string
	expectedMAC string
}

func (e *reservationConflictError) Error() string {
	return "reserved IPv4 " + e.ip + " is already present at MAC " + e.observedMAC + "; expected " + e.expectedMAC
}
