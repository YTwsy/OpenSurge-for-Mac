package gateway

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
)

func TestReservationChecksAreBoundedConcurrentAndPreserveConflicts(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameWiFiDHCP
	cfg.DevicePolicy.Bundle = &device.PolicyBundle{}
	for i := range 9 {
		cfg.DevicePolicy.Bundle.Compiled.Reservations = append(cfg.DevicePolicy.Bundle.Compiled.Reservations, device.Reservation{IPv4: fmt.Sprintf("192.168.1.%d", i+100), MAC: fmt.Sprintf("02:00:00:00:00:%02x", i)})
	}
	var mu sync.Mutex
	active, maximum := 0, 0
	seen := map[string]string{}
	entered := make(chan struct{}, 9)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	firstConflict := errors.New("first configured conflict")
	deps := gatewayDeps{probeReservationIP: func(ip, mac string) error {
		mu.Lock()
		active++
		maximum = max(maximum, active)
		seen[ip] = mac
		mu.Unlock()
		entered <- struct{}{}
		<-release
		mu.Lock()
		active--
		mu.Unlock()
		if ip == "192.168.1.101" {
			return firstConflict
		}
		if ip == "192.168.1.103" {
			return errors.New("later conflict")
		}
		return nil
	}}
	done := make(chan error, 1)
	go func() { done <- (Manager{cfg: cfg}).checkReservationConflicts(deps) }()
	for range 4 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("reservation probes are still serialized")
		}
	}
	select {
	case <-entered:
		t.Fatal("more than four reservation probes started together")
	default:
	}
	unblock()
	select {
	case err := <-done:
		if !errors.Is(err, firstConflict) {
			t.Fatalf("conflict = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reservation checks did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if maximum != 4 || len(seen) != 9 {
		t.Fatalf("concurrency=%d, checked reservations=%d", maximum, len(seen))
	}
	for _, reservation := range cfg.DevicePolicy.Bundle.Compiled.Reservations {
		if seen[reservation.IPv4] != reservation.MAC {
			t.Fatalf("missing/mismatched check for %+v", reservation)
		}
	}
}

func TestReservationChecksKeepTopologyBoundary(t *testing.T) {
	for _, mode := range []string{config.GatewayModeSameLAN, config.GatewayModeIsolatedLAN} {
		cfg := config.Default()
		cfg.Gateway.Mode = mode
		cfg.DevicePolicy.Bundle = &device.PolicyBundle{Compiled: device.CompiledPolicy{Reservations: []device.Reservation{{IPv4: "192.168.1.100"}}}}
		if err := (Manager{cfg: cfg}).checkReservationConflicts(gatewayDeps{probeReservationIP: func(string, string) error {
			t.Error("reservation probe escaped the DHCP takeover topology")
			return nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
}
