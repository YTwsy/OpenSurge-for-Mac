package device

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadLeasesRetainsReadableRowsAroundMalformedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnsmasq.leases")
	body := "2000000000 aa:bb:cc:dd:ee:01 192.168.50.101 first 01:aa\n" +
		"not-a-valid-lease\n" +
		"invalid aa:bb:cc:dd:ee:02 192.168.50.102 bad 01:aa\n" +
		"2000000000 aa:bb:cc:dd:ee:03 192.168.50.103 last 01:aa\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	clients, err := LoadLeases(path)
	if len(clients) != 2 || clients[0].IP != "192.168.50.101" || clients[1].IP != "192.168.50.103" {
		t.Fatalf("readable leases were discarded: %#v", clients)
	}
	if err == nil || !strings.Contains(err.Error(), path+":2: expected dnsmasq lease fields") || !strings.Contains(err.Error(), path+":3: invalid expiry timestamp") {
		t.Fatalf("missing partial-inventory errors: %v", err)
	}
}

func TestLoadLeasesRetainsReadableRowsOnScannerFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnsmasq.leases")
	body := "2000000000 aa:bb:cc:dd:ee:01 192.168.50.101 first 01:aa\n" + strings.Repeat("x", 70*1024)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	clients, err := LoadLeases(path)
	if len(clients) != 1 || err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("clients=%#v err=%v", clients, err)
	}
}

func TestLoadLeasesDistinguishesMissingAndUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	if clients, err := LoadLeases(filepath.Join(dir, "missing")); err != nil || len(clients) != 0 {
		t.Fatalf("missing lease file: clients=%#v err=%v", clients, err)
	}
	if clients, err := LoadLeases(dir); err == nil || len(clients) != 0 {
		t.Fatalf("unreadable lease file was reported as empty: clients=%#v err=%v", clients, err)
	}
}
