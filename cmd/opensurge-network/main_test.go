package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"open-mihomo-gateway/internal/ipv6packet"
)

func TestProbeDHCPExpectation(t *testing.T) {
	original := probeDHCP
	originalUID := effectiveUID
	defer func() { probeDHCP = original; effectiveUID = originalUID }()
	probeDHCP = func(context.Context, string, time.Duration) ([]string, error) { return []string{"192.168.1.1"}, nil }
	effectiveUID = func() int { return 0 }
	var stdout, stderr bytes.Buffer
	if code := run([]string{"probe-dhcp", "--interface", "en0", "--expect", "none"}, &stdout, &stderr); code != 3 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestProbeDHCPRequiresExpectation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"probe-dhcp", "--interface", "en0"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestIPv6PacketCommandRequiresRootAndPassesBrokerConfig(t *testing.T) {
	originalBroker := runIPv6Broker
	originalUID := effectiveUID
	t.Cleanup(func() { runIPv6Broker = originalBroker; effectiveUID = originalUID })
	socket := filepath.Join(t.TempDir(), "ipv6-packet.sock")
	ready := filepath.Join(filepath.Dir(socket), "ready")
	var got ipv6packet.BrokerConfig
	runIPv6Broker = func(_ context.Context, cfg ipv6packet.BrokerConfig) error { got = cfg; return nil }
	var stdout, stderr bytes.Buffer
	effectiveUID = func() int { return 501 }
	args := []string{"ipv6-packet", "--interface", "en7", "--socket", socket, "--ready-file", ready, "--mtu", "1420"}
	if code := run(args, &stdout, &stderr); code != 1 || !bytes.Contains(stderr.Bytes(), []byte("requires root")) {
		t.Fatalf("non-root code=%d stderr=%s", code, stderr.String())
	}
	effectiveUID = func() int { return 0 }
	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("root code=%d stderr=%s", code, stderr.String())
	}
	if got.Interface != "en7" || got.Socket != socket || got.ReadyFile != ready || got.MTU != 1420 {
		t.Fatalf("broker config = %#v", got)
	}
}
