package device

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func LoadLeases(path string) ([]Client, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func FormatClients(clients []Client) string {
	if len(clients) == 0 {
		return "No DHCP leases found.\n"
	}

	var out strings.Builder
	fmt.Fprintf(&out, "%-15s %-17s %-14s %s\n", "IP", "MAC", "HOSTNAME", "EXPIRES")
	for _, client := range clients {
		hostname := client.Hostname
		if hostname == "" {
			hostname = "*"
		}
		fmt.Fprintf(&out, "%-15s %-17s %-14s %s\n", client.IP, client.MAC, hostname, client.ExpiresAt.Format(timeFormat))
	}
	return out.String()
}

const timeFormat = "2006-01-02 15:04:05"
