package dhcp

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"open-mihomo-gateway/internal/device"
)

// ReconcilePolicyLeases removes only stale lease rows for MAC addresses owned
// by the applied policy. Unrelated dynamic leases remain untouched. This makes
// a stop/start policy update wait for a fresh ACK at the new reserved address
// instead of presenting the old address as a current identity.
func ReconcilePolicyLeases(path string, reservations []device.Reservation) error {
	if len(reservations) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	expected := make(map[string]string, len(reservations))
	for _, reservation := range reservations {
		expected[strings.ToLower(reservation.MAC)] = reservation.IPv4
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	kept := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			if mac, err := net.ParseMAC(fields[1]); err == nil {
				if want, managed := expected[strings.ToLower(mac.String())]; managed && fields[2] != want {
					changed = true
					continue
				}
			}
		}
		kept = append(kept, line)
	}
	if !changed {
		return nil
	}
	result := strings.Join(kept, "\n")
	if result != "" {
		result += "\n"
	}
	return writeLeaseFileAtomically(path, []byte(result))
}

func writeLeaseFileAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dnsmasq.leases-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o640); err != nil {
		return fmt.Errorf("chmod lease file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
