package macosnetwork

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

type dnsFixture struct {
	servers  []string
	name     string
	iface    string
	readErr  bool
	writeErr bool
	writes   int
}

func mockDNS(t *testing.T, servers []string) *dnsFixture {
	t.Helper()
	f := &dnsFixture{servers: slices.Clone(servers), name: "Wi-Fi", iface: "en0"}
	oldRun, oldSC := runCommand, runSCQuery
	t.Cleanup(func() { runCommand, runSCQuery = oldRun, oldSC })
	runCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		switch args[0] {
		case "-listnetworkserviceorder":
			return "(1) " + f.name + "\n(Hardware Port: Wi-Fi, Device: " + f.iface + ")\n", nil
		case "-getdnsservers":
			if f.readErr {
				return "", errors.New("cannot read DNS")
			}
			if len(f.servers) == 0 {
				return "There aren't any DNS Servers set on " + f.name + ".\n", nil
			}
			return strings.Join(f.servers, "\n"), nil
		case "-setdnsservers":
			if args[1] != f.name {
				t.Fatalf("wrong service: %v", args)
			}
			f.writes++
			f.servers = slices.Clone(args[2:])
			if slices.Equal(f.servers, []string{"Empty"}) {
				f.servers = nil
			}
			if f.writeErr {
				return "", errors.New("write returned error after applying")
			}
			return "", nil
		}
		t.Fatalf("unexpected networksetup command: %v", args)
		return "", nil
	}
	runSCQuery = func(_ context.Context, query string) (string, error) {
		if strings.HasPrefix(query, "list ") {
			return "subKey [0] = Setup:/Network/Service/01234567-89AB-CDEF-0123-456789ABCDEF\n", nil
		}
		if strings.HasSuffix(query, "/DNS") {
			return "<dictionary> {\n ServerAddresses : <array> {\n  0 : 192.168.1.1\n }\n SearchDomains : <array> {\n  0 : home.arpa\n }\n}\n", nil
		}
		return "<dictionary> {\n UserDefinedName : " + f.name + "\n}\n<dictionary> {\n DeviceName : " + f.iface + "\n}\n", nil
	}
	return f
}

func TestSystemDNSRestoresAutomaticAndExplicitSettings(t *testing.T) {
	for _, original := range [][]string{nil, {"192.168.1.1", "fe80::1%en0"}} {
		t.Run(strings.Join(original, ","), func(t *testing.T) {
			f := mockDNS(t, original)
			snapshot, err := (SystemDNS{}).Prepare(t.Context(), "en0")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(snapshot.Servers, original) || len(snapshot.Resolvers) == 0 || !slices.Contains(snapshot.Domains, "home.arpa") {
				t.Fatalf("snapshot: %#v", snapshot)
			}
			snapshot.Owned = true
			if err := (SystemDNS{}).Enable(t.Context(), snapshot); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(f.servers, []string{config.LocalSystemDNSServer}) {
				t.Fatal(f.servers)
			}
			f.name = "Renamed Wi-Fi"
			owned, err := (SystemDNS{}).Restore(t.Context(), snapshot)
			if err != nil || !owned || !slices.Equal(f.servers, original) {
				t.Fatalf("restore: %t %v %v", owned, err, f.servers)
			}
			writes := f.writes
			if _, err := (SystemDNS{}).Restore(t.Context(), snapshot); err != nil || f.writes != writes {
				t.Fatal("restore is not idempotent")
			}
		})
	}
}

func TestSystemDNSForeignChangesAndReadFailuresNeverGetOverwritten(t *testing.T) {
	f := mockDNS(t, nil)
	f.readErr = true
	if _, err := (SystemDNS{}).Prepare(t.Context(), "en0"); err == nil {
		t.Fatal("read failure treated as automatic DNS")
	}
	f.readErr = false
	snapshot, err := (SystemDNS{}).Prepare(t.Context(), "en0")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Owned = true
	f.servers = []string{"9.9.9.9"}
	if err := (SystemDNS{}).Enable(t.Context(), snapshot); err == nil {
		t.Fatal("preparation race was overwritten")
	}
	if owned, err := (SystemDNS{}).Restore(t.Context(), snapshot); err != nil || owned || f.writes != 0 {
		t.Fatalf("foreign state restore: %t %v writes=%d", owned, err, f.writes)
	}
	f.iface = "en9"
	if _, err := (SystemDNS{}).Restore(t.Context(), snapshot); err == nil || f.writes != 0 {
		t.Fatal("replacement service was overwritten")
	}
}

func TestSystemDNSPartialApplyCanBeRecoveredFromPersistedIntent(t *testing.T) {
	f := mockDNS(t, []string{"192.168.1.1"})
	snapshot, err := (SystemDNS{}).Prepare(t.Context(), "en0")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Owned = true
	f.writeErr = true
	if err := (SystemDNS{}).Enable(t.Context(), snapshot); err == nil {
		t.Fatal("expected ambiguous write failure")
	}
	f.writeErr = false
	if _, err := (SystemDNS{}).Restore(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.servers, snapshot.Servers) {
		t.Fatal("original DNS not restored")
	}
}

func TestMacRouteVerificationDetectsMissingIPv6Capture(t *testing.T) {
	old := runCommand
	t.Cleanup(func() { runCommand = old })
	missing := true
	runCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		destination := args[len(args)-1]
		if strings.Contains(destination, ":") && !slices.Contains(args, "-inet6") {
			t.Fatal("IPv6 route family missing")
		}
		device := "utun123"
		if missing && strings.HasPrefix(destination, "fdfe:") {
			device = "en0"
		}
		return "interface: " + device + "\n", nil
	}
	if err := VerifyMacTUNRoutes(t.Context(), "utun123"); err == nil {
		t.Fatal("missing fake IPv6 route was accepted")
	}
	missing = false
	if err := VerifyMacTUNRoutes(t.Context(), "utun123"); err != nil {
		t.Fatal(err)
	}
}
