package sysctl

import "testing"

func TestFormatForwarding(t *testing.T) {
	tests := map[string]string{
		"1": "enabled",
		"0": "disabled",
		"":  "unknown",
	}

	for input, want := range tests {
		if got := FormatForwarding(input); got != want {
			t.Fatalf("FormatForwarding(%q) = %q, want %q", input, got, want)
		}
	}
}
