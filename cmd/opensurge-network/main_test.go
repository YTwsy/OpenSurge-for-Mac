package main

import (
	"bytes"
	"context"
	"testing"
	"time"
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
