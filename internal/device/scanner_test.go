package device

import (
	"fmt"
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
