package controlapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
)

func TestConnectivityCatalogHasStableSplitRoutingTargets(t *testing.T) {
	targets := connectivityCatalog()
	if len(targets) < 12 {
		t.Fatalf("targets = %d", len(targets))
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if seen[target.ID] || target.ID == "" || target.Name == "" || target.URL == "" {
			t.Fatalf("invalid target = %#v", target)
		}
		seen[target.ID] = true
		if target.Category == "china" && target.ExpectedRoute != "direct" {
			t.Fatalf("china target does not expect direct: %#v", target)
		}
	}
}

func TestSelectedConnectivityTargetsKeepsCatalogOrderAndRejectsUnknown(t *testing.T) {
	targets, err := selectedConnectivityTargets([]string{"github", "baidu", "github"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].ID != "baidu" || targets[1].ID != "github" {
		t.Fatalf("targets = %#v", targets)
	}
	if _, err := selectedConnectivityTargets([]string{"private-target"}); err == nil {
		t.Fatal("unknown target was accepted")
	}
}

func TestConnectivityRouteAndGradeSemantics(t *testing.T) {
	for _, test := range []struct {
		chain []string
		route string
	}{
		{[]string{"DIRECT"}, "direct"},
		{[]string{"AI", "Singapore-03"}, "proxy"},
		{[]string{"REJECT"}, "reject"},
		{nil, "unknown"},
	} {
		if got := connectivityRoute(test.chain); got != test.route {
			t.Fatalf("connectivityRoute(%v) = %q", test.chain, got)
		}
	}
	if connectivityGrade(80) != "excellent" || connectivityGrade(450) != "good" || connectivityGrade(900) != "slow" || connectivityGrade(2100) != "very_slow" {
		t.Fatal("unexpected connectivity grade thresholds")
	}
}

func TestConnectivityEndpointsReturnCatalogAndSelectedResults(t *testing.T) {
	server := newTestServer(t)
	server.probeConnectivity = func(_ context.Context, _ config.Config, target ConnectivityTarget) ConnectivityResult {
		matches := true
		return ConnectivityResult{
			TargetID: target.ID, Status: "reachable", Grade: "excellent", MedianMS: 42,
			HTTPStatus: 204, Chain: []string{"DIRECT"}, Route: "direct", RouteMatch: &matches,
			Samples: []ConnectivitySample{{Status: "reachable", DelayMS: 42, HTTPStatus: 204}}, TestedAt: time.Now().UTC(),
		}
	}

	response := performAuthorized(server, http.MethodGet, "/api/v1/connectivity", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	var catalog ConnectivityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Source != "gateway_mihomo" || catalog.Scope != "applied_global_rules" || len(catalog.Targets) < 12 || len(catalog.Results) != 0 {
		t.Fatalf("catalog = %#v", catalog)
	}

	response = performAuthorized(server, http.MethodPost, "/api/v1/connectivity/tests", []byte(`{"target_ids":["baidu","github"]}`))
	if response.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", response.Code, response.Body.String())
	}
	var run ConnectivityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if len(run.Results) != 2 || run.Results[0].TargetID != "baidu" || run.Results[1].TargetID != "github" || run.CompletedAt == nil {
		t.Fatalf("run = %#v", run)
	}
}

func TestConnectivityEndpointRejectsUnknownTargets(t *testing.T) {
	server := newTestServer(t)
	response := performAuthorized(server, http.MethodPost, "/api/v1/connectivity/tests", []byte(`{"target_ids":["http://127.0.0.1"]}`))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
