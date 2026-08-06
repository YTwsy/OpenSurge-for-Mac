package device

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLeaseLine(t *testing.T) {
	expires := time.Now().Add(2 * time.Hour).Unix()
	line := fmt.Sprintf("%d aa:bb:cc:dd:ee:ff 192.168.50.101 iphone 01:aa", expires)

	client, err := parseLeaseLine(line)
	if err != nil {
		t.Fatalf("parseLeaseLine() error = %v", err)
	}
	if client.IP != "192.168.50.101" {
		t.Fatalf("IP = %q", client.IP)
	}
	if client.Hostname != "iphone" {
		t.Fatalf("Hostname = %q", client.Hostname)
	}
	if !client.Online {
		t.Fatalf("Online = false")
	}
}

func TestLoadLeasesIgnoresDNSMasqDUIDMetadata(t *testing.T) {
	expires := time.Now().Add(2 * time.Hour).Unix()
	path := filepath.Join(t.TempDir(), "dnsmasq.leases")
	body := fmt.Sprintf("%d aa:bb:cc:dd:ee:ff 192.168.50.101 iphone 01:aa\nduid 00:01:00:01:32:06:3e:58:46:1a:ff:9c:34:8c\n", expires)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	clients, err := LoadLeases(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 || clients[0].IP != "192.168.50.101" {
		t.Fatalf("clients = %#v", clients)
	}
}

func TestParseLeaseLineWildcardHostname(t *testing.T) {
	expires := time.Now().Add(time.Hour).Unix()
	line := fmt.Sprintf("%d aa:bb:cc:dd:ee:ff 192.168.50.101 * 01:aa", expires)

	client, err := parseLeaseLine(line)
	if err != nil {
		t.Fatalf("parseLeaseLine() error = %v", err)
	}
	if client.Hostname != "" {
		t.Fatalf("Hostname = %q", client.Hostname)
	}
}
