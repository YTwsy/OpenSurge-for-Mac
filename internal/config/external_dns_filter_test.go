package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExternalDNSFilterValidationAndPersistence(t *testing.T) {
	cfg := Default()
	if cfg.ExternalDNSFilter.Enabled {
		t.Fatal("must be opt in")
	}
	cfg.Transparent.Mode = TransparentModeTUN
	cfg.ExternalDNSFilter = ExternalDNSFilterConfig{Enabled: true, Server: "127.0.0.1:5353", TimeoutMS: 1200, BlockedAddresses: []string{"192.0.2.99", "::1"}, BypassDomains: []string{"Private.Example."}}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(Render(cfg)), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || !reflect.DeepEqual(loaded.ExternalDNSFilter, cfg.ExternalDNSFilter) {
		t.Fatalf("round trip: %+v %v", loaded.ExternalDNSFilter, err)
	}
	for _, server := range []string{"filter.example:53", "127.0.0.1:1053", "127.0.0.1:53", "192.168.50.1:53", "0.0.0.0:5353", "[::]:5353", "198.18.1.1:53", "224.0.0.1:53", "127.0.0.1:0", "[fe80::1%en0]:53"} {
		candidate := cfg
		candidate.ExternalDNSFilter.Server = server
		if err := Validate(candidate); err == nil {
			t.Errorf("accepted %s", server)
		}
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.ExternalDNSFilter.TimeoutMS = 0 },
		func(c *Config) { c.ExternalDNSFilter.BypassDomains = []string{"*.example"} },
		func(c *Config) { c.ExternalDNSFilter.BlockedAddresses = []string{"example"} },
		func(c *Config) { c.DNS.Upstream = "1.1.1.1" },
	} {
		candidate := cfg
		change(&candidate)
		if err := Validate(candidate); err == nil {
			t.Error("invalid filter configuration accepted")
		}
	}
	legacy := strings.Split(Render(Default()), "external_dns_filter:")
	data := legacy[0] + "mihomo:" + strings.SplitN(legacy[1], "mihomo:", 2)[1]
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil || loaded.ExternalDNSFilter.Enabled || loaded.ExternalDNSFilter.TimeoutMS != 2000 {
		t.Fatalf("legacy config changed: %+v %v", loaded.ExternalDNSFilter, err)
	}
}
