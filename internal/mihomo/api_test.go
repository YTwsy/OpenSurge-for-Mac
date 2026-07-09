package mihomo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestFetchVersion(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.APIAddr = "127.0.0.1:9090"
	cfg.Mihomo.Secret = "test-secret"

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://127.0.0.1:9090/version" {
			t.Fatalf("URL = %q", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"version":"v1.2.3","meta":true}`)),
			Header:     make(http.Header),
		}, nil
	})}

	version, err := fetchVersionWithClient(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("fetchVersionWithClient() error = %v", err)
	}
	if version.Version != "v1.2.3" {
		t.Fatalf("Version = %q", version.Version)
	}
	if !version.Meta {
		t.Fatalf("Meta = false")
	}
}

func TestFetchProxyGroups(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.APIAddr = "127.0.0.1:9090"
	cfg.Mihomo.Secret = "test-secret"

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://127.0.0.1:9090/proxies" {
			t.Fatalf("URL = %q", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body: io.NopCloser(strings.NewReader(`{
			  "proxies": {
			    "DIRECT": {"name":"DIRECT","type":"Direct"},
			    "Proxy": {"name":"Proxy","type":"Selector","now":"HK","all":["DIRECT","HK"]},
			    "Fallback": {"type":"Fallback","now":"JP","all":["JP","US"]}
			  }
			}`)),
			Header: make(http.Header),
		}, nil
	})}

	groups, err := fetchProxyGroupsWithClient(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("fetchProxyGroupsWithClient() error = %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2: %#v", len(groups), groups)
	}
	if groups[0].Name != "Fallback" || groups[0].Selected != "JP" || strings.Join(groups[0].Options, ",") != "JP,US" {
		t.Fatalf("groups[0] = %#v", groups[0])
	}
	if groups[1].Name != "Proxy" || groups[1].Selected != "HK" || strings.Join(groups[1].Options, ",") != "DIRECT,HK" {
		t.Fatalf("groups[1] = %#v", groups[1])
	}
}

func TestSelectProxyGroup(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.APIAddr = "http://127.0.0.1:9090/"
	cfg.Mihomo.Secret = "test-secret"

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPut {
			t.Fatalf("Method = %q", req.Method)
		}
		if req.URL.String() != "http://127.0.0.1:9090/proxies/Proxy%20Group" {
			t.Fatalf("URL = %q", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["name"] != "DIRECT" {
			t.Fatalf("body name = %q", body["name"])
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Status:     "204 No Content",
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})}

	if err := selectProxyGroupWithClient(context.Background(), cfg, client, "Proxy Group", "DIRECT"); err != nil {
		t.Fatalf("selectProxyGroupWithClient() error = %v", err)
	}
}

func TestFetchConnections(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.APIAddr = "127.0.0.1:9090"
	cfg.Mihomo.Secret = "test-secret"

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://127.0.0.1:9090/connections" {
			t.Fatalf("URL = %q", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body: io.NopCloser(strings.NewReader(`{
			  "downloadTotal": 2048,
			  "uploadTotal": 1024,
			  "connections": [
			    {
			      "id": "abc",
			      "upload": 10,
			      "download": 20,
			      "start": "2026-07-09T10:00:00Z",
			      "chains": ["Proxy", "demo-proxy"],
			      "rule": "Domain",
			      "rulePayload": "example.com",
			      "metadata": {
			        "host": "example.com",
			        "destinationIP": "93.184.216.34",
			        "destinationPort": "443"
			      }
			    }
			  ]
			}`)),
			Header: make(http.Header),
		}, nil
	})}

	snapshot, err := fetchConnectionsWithClient(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("fetchConnectionsWithClient() error = %v", err)
	}
	if snapshot.UploadTotal != 1024 || snapshot.DownloadTotal != 2048 {
		t.Fatalf("totals = %d/%d", snapshot.UploadTotal, snapshot.DownloadTotal)
	}
	if len(snapshot.Connections) != 1 {
		t.Fatalf("connections = %#v", snapshot.Connections)
	}
	connection := snapshot.Connections[0]
	if connection.ID != "abc" || connection.RulePayload != "example.com" || strings.Join(connection.Chains, ",") != "Proxy,demo-proxy" {
		t.Fatalf("connection = %#v", connection)
	}
	if connection.Metadata["host"] != "example.com" {
		t.Fatalf("metadata = %#v", connection.Metadata)
	}
}

func TestImportedProfilePolicySwitchFixture(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join("..", "..", "examples", "mihomo-profile.example.yaml")
	cfg.Mihomo.APIAddr = "127.0.0.1:9090"

	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		"profile:",
		"  store-selected: true",
		"- name: \"Proxy\"",
		"- \"demo-proxy\"",
		"- DIRECT",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered imported fixture missing %q:\n%s", want, rendered)
		}
	}

	selected := "demo-proxy"
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/proxies":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body: io.NopCloser(strings.NewReader(`{
				  "proxies": {
				    "DIRECT": {"name":"DIRECT","type":"Direct"},
				    "demo-proxy": {"name":"demo-proxy","type":"Http"},
				    "Proxy": {"name":"Proxy","type":"Selector","now":"` + selected + `","all":["demo-proxy","DIRECT"]}
				  }
				}`)),
				Header: make(http.Header),
			}, nil
		case req.Method == http.MethodPut && req.URL.EscapedPath() == "/proxies/Proxy":
			var body map[string]string
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			selected = body["name"]
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     "204 No Content",
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		default:
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}

	groups, err := fetchProxyGroupsWithClient(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("initial fetchProxyGroupsWithClient() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Proxy" || groups[0].Selected != "demo-proxy" {
		t.Fatalf("initial groups = %#v", groups)
	}

	if err := selectProxyGroupWithClient(context.Background(), cfg, client, "Proxy", "DIRECT"); err != nil {
		t.Fatalf("selectProxyGroupWithClient() error = %v", err)
	}

	groups, err = fetchProxyGroupsWithClient(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("second fetchProxyGroupsWithClient() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Selected != "DIRECT" {
		t.Fatalf("selected groups = %#v", groups)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
