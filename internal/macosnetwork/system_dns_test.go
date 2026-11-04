package macosnetwork

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

type dnsFixture struct {
	servers  []string
	id       string
	name     string
	iface    string
	readErr  bool
	writeErr bool
	writes   int
}

func mockDNS(t *testing.T, servers []string) *dnsFixture {
	t.Helper()
	f := &dnsFixture{servers: slices.Clone(servers), id: "01234567-89AB-CDEF-0123-456789ABCDEF", name: "Wi-Fi", iface: "en0"}
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
			return "subKey [0] = Setup:/Network/Service/" + f.id + "\n", nil
		}
		if strings.HasSuffix(query, "/DNS") {
			return "<dictionary> {\n ServerAddresses : <array> {\n  0 : 192.168.1.1\n }\n SearchDomains : <array> {\n  0 : home.arpa\n }\n}\n", nil
		}
		if !strings.HasPrefix(query, "show Setup:/Network/Service/"+f.id+"\n") {
			return "  No such key\n  No such key\n", nil
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
			if _, err := (SystemDNS{}).Restore(t.Context(), snapshot); err != nil || !slices.Equal(f.servers, original) {
				t.Fatal("repeated restore changed the saved DNS settings")
			}
		})
	}
}

func TestSystemDNSUsesBackupDespiteInterveningChanges(t *testing.T) {
	for _, original := range [][]string{nil, {"192.168.1.1", "fe80::1%en0"}} {
		t.Run(strings.Join(original, ","), func(t *testing.T) {
			f, snapshot := savedDNSFixture(t, original)
			f.servers = []string{"9.9.9.9"}
			if err := (SystemDNS{}).Enable(t.Context(), snapshot); err != nil {
				t.Fatal("intervening DNS change blocked takeover", err)
			}
			if !slices.Equal(f.servers, []string{config.LocalSystemDNSServer}) {
				t.Fatal("takeover was not applied", f.servers)
			}
			// Restore the saved settings even if another actor changed DNS later.
			f.servers = []string{"8.8.8.8"}
			present, err := (SystemDNS{}).Restore(t.Context(), snapshot)
			if err != nil || !present || !slices.Equal(f.servers, original) {
				t.Fatalf("backup restore: present=%t err=%v servers=%v", present, err, f.servers)
			}
		})
	}
}

func TestSystemDNSPrepareReadFailureDoesNotBecomeAutomaticBackup(t *testing.T) {
	f := mockDNS(t, nil)
	f.readErr = true
	if _, err := (SystemDNS{}).Prepare(t.Context(), "en0"); err == nil || f.writes != 0 {
		t.Fatal("read failure treated as automatic DNS", err)
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

func savedDNSFixture(t *testing.T, original []string) (*dnsFixture, runtime.SystemDNSSnapshot) {
	t.Helper()
	f := mockDNS(t, original)
	snapshot, err := (SystemDNS{}).Prepare(t.Context(), "en0")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Owned = true
	return f, snapshot
}

func TestSystemDNSRestoreSkipsRemovedServiceWithoutRebinding(t *testing.T) {
	for _, tt := range []struct {
		name    string
		newID   string
		current []string
	}{
		{name: "service removed"},
		{name: "replacement with automatic DNS", newID: "11111111-2222-3333-4444-555555555555"},
		{name: "replacement with external DNS", newID: "11111111-2222-3333-4444-555555555555", current: []string{"9.9.9.9"}},
		{name: "replacement with managed DNS", newID: "11111111-2222-3333-4444-555555555555", current: []string{config.LocalSystemDNSServer}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, snapshot := savedDNSFixture(t, []string{"192.168.1.1"})
			f.id, f.servers = tt.newID, slices.Clone(tt.current)
			present, err := (SystemDNS{}).Restore(t.Context(), snapshot)
			if err != nil || present || f.writes != 0 || !slices.Equal(f.servers, tt.current) {
				t.Fatalf("removed service restore: present=%t err=%v writes=%d servers=%v", present, err, f.writes, f.servers)
			}
		})
	}
}

func TestSystemDNSSameUUIDAllowsServiceRenameAndInterfaceChange(t *testing.T) {
	f, snapshot := savedDNSFixture(t, []string{"192.168.1.1"})
	f.name, f.iface = "Renamed Wi-Fi", "en9"
	if err := (SystemDNS{}).Enable(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := (SystemDNS{}).Verify(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	present, err := (SystemDNS{}).Restore(t.Context(), snapshot)
	if err != nil || !present || !slices.Equal(f.servers, snapshot.Servers) {
		t.Fatalf("same service restore: present=%t err=%v servers=%v", present, err, f.servers)
	}
}

func TestSystemDNSWriteAndReadbackFailuresAreReported(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*dnsFixture)
	}{
		{name: "write failure", mutate: func(f *dnsFixture) { f.writeErr = true }},
		{name: "readback failure", mutate: func(f *dnsFixture) { f.readErr = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, snapshot := savedDNSFixture(t, []string{"192.168.1.1"})
			tt.mutate(f)
			if err := (SystemDNS{}).Enable(t.Context(), snapshot); err == nil {
				t.Fatal("takeover failure was not reported")
			}
			present, err := (SystemDNS{}).Restore(t.Context(), snapshot)
			if err == nil || !present || f.writes != 2 {
				t.Fatalf("restore failure: present=%t err=%v writes=%d", present, err, f.writes)
			}
		})
	}
}

func TestSystemDNSServiceReadErrorIsReported(t *testing.T) {
	f, snapshot := savedDNSFixture(t, nil)
	runSCQuery = func(context.Context, string) (string, error) {
		return "", errors.New("cannot read SystemConfiguration")
	}
	if reapply, err := (SystemDNS{}).Restore(t.Context(), snapshot); err == nil || !reapply || f.writes != 0 {
		t.Fatal("service read failure was hidden or prevented a later retry", reapply, err)
	}
}

func TestSystemDNSEnableAndVerifyReportMissingService(t *testing.T) {
	f, snapshot := savedDNSFixture(t, nil)
	f.id = "11111111-2222-3333-4444-555555555555"
	if err := (SystemDNS{}).Enable(t.Context(), snapshot); err == nil {
		t.Fatal("Enable reported success without its target service")
	}
	if err := (SystemDNS{}).Verify(t.Context(), snapshot); err == nil {
		t.Fatal("Verify reported success without its target service")
	}
	if f.writes != 0 {
		t.Fatal("wrote DNS to a different service")
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
	if err := VerifyMacTUNRoutes(t.Context(), "utun123", true); err == nil {
		t.Fatal("missing fake IPv6 route was accepted")
	}
	missing = false
	if err := VerifyMacTUNRoutes(t.Context(), "utun123", true); err != nil {
		t.Fatal(err)
	}
}

func TestMacTUNRoutesIPv4OnlyReadiness(t *testing.T) {
	old := runCommand
	t.Cleanup(func() { runCommand = old })
	calls := 0
	runCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		destination := args[len(args)-1]
		if strings.Contains(destination, ":") {
			t.Fatal("disabled IPv6 must not block startup", destination)
		}
		calls++
		return "interface: utun123\n", nil
	}
	if err := VerifyMacTUNRoutes(t.Context(), "utun123", false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("IPv4 readiness checks = %d", calls)
	}
}
