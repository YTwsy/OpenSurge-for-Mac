package mihomo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestRenderConfigAddsLocalRoutingBeforeImportedAndDeviceRules(t *testing.T) {
	dir := t.TempDir()
	profilePath := dir + "/profile.yaml"
	policyPath := dir + "/devices.json"
	profile := `proxies:
  - name: edge
    type: http
    server: 127.0.0.1
    port: 18080
proxy-providers:
  subscription:
    type: inline
    payload:
      - name: provider-edge
        type: http
        server: 127.0.0.1
        port: 18081
proxy-groups:
  - name: Main
    type: select
    proxies: [edge]
rules:
  - DOMAIN,global.example,Main
  - MATCH,DIRECT
`
	policy := `{
  "profiles":[{"id":"home","default_policies":["DIRECT"]}],
  "devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home","egress_mode":"dedicated"}]
}`
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	cfg.DevicePolicy.File = policyPath

	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`name: open-surge/mac-global`,
		`proxies:`,
		`- "edge"`,
		`- "Main"`,
		`use:`,
		`- "subscription"`,
		`name: open-surge/mac-mode-tcp`,
		`name: open-surge/mac-mode-udp`,
		`hidden: true`,
		`AND,((IN-TYPE,TUN),(SRC-IP-CIDR,198.18.0.1/32),(NETWORK,TCP)),open-surge/mac-mode-tcp`,
		`AND,((IN-TYPE,SOCKS/HTTP),(SRC-IP-CIDR,127.0.0.0/8),(NETWORK,UDP)),open-surge/mac-mode-udp`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	assertOrdered(t, rendered,
		"open-surge/mac-mode-tcp",
		"device/phone/default",
	)
	assertOrdered(t, rendered,
		"SRC-IP-CIDR,198.18.0.1/32",
		"SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
		"DOMAIN,global.example,Main",
		"MATCH,DIRECT",
	)
}

func TestSetLocalRoutingKeepsUnsupportedGlobalUDPFailClosed(t *testing.T) {
	api := newLocalRoutingTestAPI(t)
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Mihomo.APIAddr = api.URL

	snapshot, err := SetLocalRouting(t.Context(), cfg, LocalRoutingModeGlobal, "edge-http")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != LocalRoutingModeGlobal || snapshot.UDPBehavior != "reject" || !snapshot.Consistent {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	api.mu.Lock()
	if api.selected[LocalRoutingTCPGroup] != LocalRoutingGlobalGroup || api.selected[LocalRoutingUDPGroup] != "REJECT" || api.selected[LocalRoutingGlobalGroup] != "edge-http" {
		t.Fatalf("selected = %#v", api.selected)
	}
	api.mu.Unlock()

	snapshot, err = SetLocalRouting(t.Context(), cfg, LocalRoutingModeRule, "")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != LocalRoutingModeRule || snapshot.UDPBehavior != "rules" {
		t.Fatalf("rule snapshot = %#v", snapshot)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.selected[LocalRoutingTCPGroup] != "PASS" || api.selected[LocalRoutingUDPGroup] != "PASS" {
		t.Fatalf("rule selected = %#v", api.selected)
	}
}

func TestSetLocalRoutingUsesProxyForSupportedGlobalUDP(t *testing.T) {
	api := newLocalRoutingTestAPI(t)
	cfg := config.Default()
	cfg.Mihomo.APIAddr = api.URL

	snapshot, err := SetLocalRouting(t.Context(), cfg, LocalRoutingModeGlobal, "edge-udp")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.UDPBehavior != "proxy" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.selected[LocalRoutingUDPGroup] != LocalRoutingGlobalGroup {
		t.Fatalf("selected = %#v", api.selected)
	}
}

func TestSetLocalRoutingRollsBackPartialSelection(t *testing.T) {
	api := newLocalRoutingTestAPI(t)
	api.failGroup = LocalRoutingUDPGroup
	api.failTarget = LocalRoutingGlobalGroup
	cfg := config.Default()
	cfg.Mihomo.APIAddr = api.URL

	if _, err := SetLocalRouting(t.Context(), cfg, LocalRoutingModeGlobal, "edge-udp"); err == nil {
		t.Fatal("SetLocalRouting() succeeded despite rejected UDP selector update")
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.selected[LocalRoutingGlobalGroup] != "edge-http" ||
		api.selected[LocalRoutingTCPGroup] != "PASS" ||
		api.selected[LocalRoutingUDPGroup] != "PASS" {
		t.Fatalf("partial selections were not rolled back: %#v", api.selected)
	}
}

func TestVisibleProxyGroupsHidesLocalRoutingInternals(t *testing.T) {
	groups := VisibleProxyGroups([]ProxyGroup{
		{Name: LocalRoutingTCPGroup},
		{Name: "Main", Selected: LocalRoutingGlobalGroup, Options: []string{"DIRECT", LocalRoutingGlobalGroup}},
		{Name: LocalRoutingGlobalGroup},
		{Name: "device/phone/default"},
	})
	if len(groups) != 2 || groups[0].Name != "Main" || groups[1].Name != "device/phone/default" {
		t.Fatalf("groups = %#v", groups)
	}
	if groups[0].Selected != "" || len(groups[0].Options) != 1 || groups[0].Options[0] != "DIRECT" {
		t.Fatalf("internal local routing selection leaked through visible group: %#v", groups[0])
	}
}

func TestVisibleProxyHealthHidesLocalRoutingInternals(t *testing.T) {
	proxies := VisibleProxyHealth([]ProxyHealth{
		{Name: LocalRoutingTCPGroup},
		{Name: "GLOBAL", Selected: LocalRoutingGlobalGroup},
		{Name: "edge"},
	})
	if len(proxies) != 2 || proxies[0].Name != "GLOBAL" || proxies[0].Selected != "" || proxies[1].Name != "edge" {
		t.Fatalf("proxies = %#v", proxies)
	}
}

func TestVisibleProvidersHidesLocalRoutingInternals(t *testing.T) {
	snapshot := VisibleProviders(ProvidersSnapshot{ProxyProviders: []ProxyProvider{
		{Name: LocalRoutingGlobalGroup, ProxyCount: 1, Proxies: []ProviderProxy{{Name: "edge"}}},
		{Name: "default", ProxyCount: 3, Proxies: []ProviderProxy{{Name: "DIRECT"}, {Name: LocalRoutingTCPGroup}, {Name: "edge"}}},
	}})
	if len(snapshot.ProxyProviders) != 1 ||
		snapshot.ProxyProviders[0].Name != "default" ||
		snapshot.ProxyProviders[0].ProxyCount != 2 ||
		len(snapshot.ProxyProviders[0].Proxies) != 2 {
		t.Fatalf("providers = %#v", snapshot.ProxyProviders)
	}
}

type localRoutingTestAPI struct {
	*httptest.Server
	mu         sync.Mutex
	selected   map[string]string
	options    map[string][]string
	udp        map[string]bool
	failGroup  string
	failTarget string
	failed     bool
}

func newLocalRoutingTestAPI(t *testing.T) *localRoutingTestAPI {
	t.Helper()
	api := &localRoutingTestAPI{
		selected: map[string]string{
			LocalRoutingGlobalGroup: "edge-http",
			LocalRoutingTCPGroup:    "PASS",
			LocalRoutingUDPGroup:    "PASS",
		},
		options: map[string][]string{
			LocalRoutingGlobalGroup: {"edge-http", "edge-udp"},
			LocalRoutingTCPGroup:    {"PASS", "DIRECT", LocalRoutingGlobalGroup},
			LocalRoutingUDPGroup:    {"PASS", "DIRECT", "REJECT", LocalRoutingGlobalGroup},
		},
		udp: map[string]bool{
			"edge-http": false,
			"edge-udp":  true,
			"PASS":      true,
			"DIRECT":    true,
			"REJECT":    true,
		},
	}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		if r.Method == http.MethodGet && r.URL.Path == "/proxies" {
			proxies := map[string]any{}
			for name, options := range api.options {
				selected := api.selected[name]
				udp := api.udp[selected]
				if name == LocalRoutingGlobalGroup {
					udp = api.udp[selected]
				}
				proxies[name] = map[string]any{"name": name, "type": "Selector", "now": selected, "all": options, "udp": udp}
			}
			for name, udp := range api.udp {
				proxies[name] = map[string]any{"name": name, "type": "HTTP", "now": "", "all": []string{}, "udp": udp}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"proxies": proxies})
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/proxies/") {
			name, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/proxies/"))
			var request struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !containsString(api.options[name], request.Name) {
				http.Error(w, "invalid selection", http.StatusUnprocessableEntity)
				return
			}
			if !api.failed && name == api.failGroup && request.Name == api.failTarget {
				api.failed = true
				http.Error(w, "injected failure", http.StatusInternalServerError)
				return
			}
			api.selected[name] = request.Name
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(api.Close)
	return api
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
