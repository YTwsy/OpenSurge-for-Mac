package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestStatusFormatLabelsDNSOnlyMode(t *testing.T) {
	status := Status{
		Gateway:   "running",
		Interface: "en0",
		LANIP:     "192.168.1.20",
		DHCP:      "running",
	}

	got := status.Format()
	if !strings.Contains(got, "DNS: running") {
		t.Fatalf("status did not label DNS-only mode:\n%s", got)
	}
	if strings.Contains(got, "DHCP: running") {
		t.Fatalf("status incorrectly labeled DNS-only mode as DHCP:\n%s", got)
	}
}

func TestStatusFormatLabelsDHCPMode(t *testing.T) {
	status := Status{
		Gateway:     "running",
		Interface:   "en7",
		LANIP:       "192.168.50.1",
		DHCP:        "running",
		DHCPEnabled: true,
	}

	got := status.Format()
	if !strings.Contains(got, "DHCP: running") {
		t.Fatalf("status did not preserve DHCP label:\n%s", got)
	}
}

func TestStatusFormatIncludesTUNInterfaceAndError(t *testing.T) {
	status := Status{
		TUN:          "failed",
		TUNInterface: "utun7",
		TUNError:     "mihomo runtime config reports TUN disabled",
	}
	got := status.Format()
	if !strings.Contains(got, "TUN: failed (utun7): mihomo runtime config reports TUN disabled") {
		t.Fatalf("status did not expose TUN failure details:\n%s", got)
	}
}

func TestStatusDegradesWhenRunningMihomoReportsTUNDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "v1.19.27", "meta": true})
		case "/configs":
			_ = json.NewEncoder(w).Encode(map[string]any{"tun": map[string]any{"enable": false, "device": "utun123"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	cfg.Mihomo.APIAddr = server.URL
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.DHCP.Enabled = true
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDMihomo: os.Getpid(), PIDDNSMasq: os.Getpid()}); err != nil {
		t.Fatal(err)
	}

	status, err := New(cfg).Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Gateway != "degraded" || status.TUN != "failed" || status.TUNInterface != "utun123" || status.TUNError == "" {
		t.Fatalf("status = %#v", status)
	}
}
