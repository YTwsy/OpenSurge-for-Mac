package device

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompilePolicySetCreatesIndependentDeviceGroupsAndRules(t *testing.T) {
	set := PolicySet{
		Templates: []Template{{
			ID:              "base",
			DefaultPolicies: []string{"DIRECT", "Global"},
			Rules: []Rule{{
				ID:     "block-udp",
				Match:  RuleMatch{Protocols: []string{"udp"}},
				Action: "REJECT",
			}},
		}},
		RuleSets: []RuleSet{{
			ID:       "streaming",
			Behavior: "domain",
			Payload:  []string{"netflix.com", "youtube.com"},
		}},
		Profiles: []Profile{{
			ID:       "household",
			Template: "base",
			Rules: []Rule{{
				ID:       "streaming",
				Match:    RuleMatch{RuleSets: []string{"streaming"}, Protocols: []string{"tcp"}},
				Policies: []string{"Global", "DIRECT"},
			}},
		}},
		Devices: []ManagedDevice{
			{ID: "phone", MAC: "AA:BB:CC:DD:EE:01", IPv4: "192.168.50.101", Profile: "household"},
			{ID: "tablet", MAC: "aa:bb:cc:dd:ee:02", IPv4: "192.168.50.102", Profile: "household"},
		},
	}

	compiled, err := CompilePolicySet(set)
	if err != nil {
		t.Fatalf("CompilePolicySet() error = %v", err)
	}
	if len(compiled.Reservations) != 2 || compiled.Reservations[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("reservations = %#v", compiled.Reservations)
	}
	for _, want := range []string{
		"device/phone/default",
		"device/phone/streaming",
		"device/tablet/default",
		"device/tablet/streaming",
	} {
		if !hasSelectorGroup(compiled.SelectorGroups, want) {
			t.Fatalf("selector groups missing %q: %#v", want, compiled.SelectorGroups)
		}
	}
	for _, want := range []string{
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(NETWORK,udp)),REJECT",
		"AND,((SRC-IP-CIDR,192.168.50.102/32),(NETWORK,udp)),REJECT",
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(NETWORK,tcp),(RULE-SET,open-surge-ruleset-streaming)),device/phone/streaming",
		"AND,((SRC-IP-CIDR,192.168.50.102/32),(NETWORK,tcp),(RULE-SET,open-surge-ruleset-streaming)),device/tablet/streaming",
	} {
		if !contains(compiled.OverrideRules, want) {
			t.Fatalf("override rules missing %q:\n%s", want, strings.Join(compiled.OverrideRules, "\n"))
		}
	}
	if !contains(compiled.DefaultRules, "SRC-IP-CIDR,192.168.50.101/32,device/phone/default") ||
		!contains(compiled.DefaultRules, "SRC-IP-CIDR,192.168.50.102/32,device/tablet/default") {
		t.Fatalf("default rules = %#v", compiled.DefaultRules)
	}
	if len(compiled.RuleProviders) != 1 || compiled.RuleProviders[0].Name != "open-surge-ruleset-streaming" || compiled.RuleProviders[0].Type != "inline" {
		t.Fatalf("providers = %#v", compiled.RuleProviders)
	}
	if group, err := DeviceGroup(set, "phone", "streaming"); err != nil || group != "device/phone/streaming" {
		t.Fatalf("DeviceGroup() = %q, %v", group, err)
	}
}

func TestPolicySetValidationRejectsUnsafeOrAmbiguousPolicies(t *testing.T) {
	base := PolicySet{
		Profiles: []Profile{{ID: "default", DefaultPolicies: []string{"DIRECT"}}},
		Devices:  []ManagedDevice{{ID: "phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.50.101", Profile: "default"}},
	}
	tests := []struct {
		name string
		edit func(*PolicySet)
		want string
	}{
		{
			name: "duplicate ipv4",
			edit: func(set *PolicySet) {
				set.Devices = append(set.Devices, ManagedDevice{ID: "tablet", MAC: "aa:bb:cc:dd:ee:02", IPv4: "192.168.50.101", Profile: "default"})
			},
			want: "duplicate device ipv4",
		},
		{
			name: "unknown profile",
			edit: func(set *PolicySet) { set.Devices[0].Profile = "missing" },
			want: "unknown profile",
		},
		{
			name: "rule action and selector",
			edit: func(set *PolicySet) {
				set.Profiles[0].Rules = []Rule{{ID: "bad", Match: RuleMatch{Domains: []string{"example.com"}}, Action: "DIRECT", Policies: []string{"DIRECT"}}}
			},
			want: "cannot set action",
		},
		{
			name: "classical mrs",
			edit: func(set *PolicySet) {
				set.RuleSets = []RuleSet{{ID: "bad", Type: "http", Behavior: "classical", Format: "mrs", URL: "https://example.com/rules.mrs"}}
			},
			want: "mrs format supports domain or ipcidr",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := clonePolicySet(base)
			test.edit(&set)
			err := ValidatePolicySet(set)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidatePolicySet() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadPolicySetRejectsUnknownJSONFieldsAndValidatesLAN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	body := `{
  "devices": [{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"default"}],
  "profiles": [{"id":"default","default_policies":["DIRECT"]}]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := LoadPolicySet(path)
	if err != nil {
		t.Fatalf("LoadPolicySet() error = %v", err)
	}
	if err := ValidatePolicySetForLAN(set, "192.168.50.1"); err != nil {
		t.Fatalf("ValidatePolicySetForLAN() error = %v", err)
	}
	if err := ValidatePolicySetForLAN(set, "192.168.51.1"); err == nil || !strings.Contains(err.Error(), "must remain in gateway LAN") {
		t.Fatalf("ValidatePolicySetForLAN() error = %v", err)
	}
	set.Devices[0].IPv4 = "192.168.50.255"
	if err := ValidatePolicySetForLAN(set, "192.168.50.1"); err == nil || !strings.Contains(err.Error(), "network or broadcast") {
		t.Fatalf("ValidatePolicySetForLAN() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"unknown":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicySet(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadPolicySet() error = %v", err)
	}
}

func TestStarterPolicyExampleIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "device-policy.example.json")
	set, err := LoadPolicySet(path)
	if err != nil {
		t.Fatalf("LoadPolicySet(%q) error = %v", path, err)
	}
	if len(set.Devices) != 0 || len(set.Profiles) != 0 || len(set.Templates) != 0 || len(set.RuleSets) != 0 {
		t.Fatalf("starter policy = %#v, want an empty valid policy set", set)
	}
}

func TestCompilePolicySetPreservesHTTPMRSRuleProviderWithoutFetchingIt(t *testing.T) {
	set := PolicySet{
		RuleSets: []RuleSet{{
			ID:       "large-domain-list",
			Type:     "http",
			Behavior: "domain",
			Format:   "mrs",
			URL:      "https://rules.example.test/large-domain-list.mrs",
			Interval: 3600,
		}},
		Profiles: []Profile{{
			ID:              "default",
			DefaultPolicies: []string{"DIRECT"},
			Rules: []Rule{{
				ID:     "large-list",
				Match:  RuleMatch{RuleSets: []string{"large-domain-list"}},
				Action: "DIRECT",
			}},
		}},
		Devices: []ManagedDevice{{
			ID: "phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.50.101", Profile: "default",
		}},
	}

	compiled, err := CompilePolicySet(set)
	if err != nil {
		t.Fatalf("CompilePolicySet() error = %v", err)
	}
	if len(compiled.RuleProviders) != 1 {
		t.Fatalf("providers = %#v", compiled.RuleProviders)
	}
	provider := compiled.RuleProviders[0]
	if provider.Name != "open-surge-ruleset-large-domain-list" || provider.Type != "http" || provider.Behavior != "domain" || provider.Format != "mrs" || provider.Interval != 3600 {
		t.Fatalf("provider = %#v", provider)
	}
	if !contains(compiled.OverrideRules, "AND,((SRC-IP-CIDR,192.168.50.101/32),(RULE-SET,open-surge-ruleset-large-domain-list)),DIRECT") {
		t.Fatalf("rules = %#v", compiled.OverrideRules)
	}
}

func hasSelectorGroup(groups []SelectorGroup, name string) bool {
	for _, group := range groups {
		if group.Name == name {
			return true
		}
	}
	return false
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func clonePolicySet(set PolicySet) PolicySet {
	copySet := set
	copySet.Devices = append([]ManagedDevice(nil), set.Devices...)
	copySet.Profiles = append([]Profile(nil), set.Profiles...)
	for i := range copySet.Profiles {
		copySet.Profiles[i].DefaultPolicies = append([]string(nil), set.Profiles[i].DefaultPolicies...)
		copySet.Profiles[i].Rules = append([]Rule(nil), set.Profiles[i].Rules...)
	}
	copySet.RuleSets = append([]RuleSet(nil), set.RuleSets...)
	return copySet
}
