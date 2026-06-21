package device

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func LoadLeases(path string) ([]Client, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var clients []Client
	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		client, err := parseLeaseLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		clients = append(clients, client)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return clients, nil
}

func parseLeaseLine(line string) (Client, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return Client{}, fmt.Errorf("expected dnsmasq lease fields")
	}
	expiresUnix, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return Client{}, fmt.Errorf("invalid expiry timestamp")
	}
	hostname := fields[3]
	if hostname == "*" {
		hostname = ""
	}
	expiresAt := time.Unix(expiresUnix, 0)
	return Client{
		IP:        fields[2],
		MAC:       fields[1],
		Hostname:  hostname,
		ExpiresAt: expiresAt,
		Online:    expiresAt.After(time.Now()),
	}, nil
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
		fmt.Fprintf(&out, "%-15s %-17s %-14s %s\n", client.IP, client.MAC, hostname, formatRemaining(client.ExpiresAt))
	}
	return out.String()
}

func formatRemaining(expiresAt time.Time) string {
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return "expired"
	}
	remaining = remaining.Round(time.Minute)
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
