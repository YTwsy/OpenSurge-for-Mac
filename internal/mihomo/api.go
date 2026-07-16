package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
)

type Version struct {
	Version string `json:"version"`
	Meta    bool   `json:"meta"`
}

type ProxyGroup struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Selected string   `json:"selected"`
	Options  []string `json:"options"`
}

const DefaultProxyDelayTestURL = "https://www.gstatic.com/generate_204"

type ProxyHealthSnapshot struct {
	TestURL string        `json:"test_url"`
	Proxies []ProxyHealth `json:"proxies"`
}

type ProxyHealth struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Selected  string `json:"selected,omitempty"`
	Provider  string `json:"provider,omitempty"`
	UDP       bool   `json:"udp"`
	Status    string `json:"status"`
	DelayMS   int    `json:"delay_ms,omitempty"`
	TestedAt  string `json:"tested_at,omitempty"`
	Probeable bool   `json:"probeable"`
}

type ProxyDelayResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	DelayMS  int    `json:"delay_ms,omitempty"`
	TestedAt string `json:"tested_at"`
	TestURL  string `json:"test_url"`
	Error    string `json:"error,omitempty"`
}

type ConnectionsSnapshot struct {
	UploadTotal   int64        `json:"upload_total"`
	DownloadTotal int64        `json:"download_total"`
	Connections   []Connection `json:"connections"`
}

type Connection struct {
	ID          string         `json:"id"`
	Upload      int64          `json:"upload"`
	Download    int64          `json:"download"`
	Start       string         `json:"start,omitempty"`
	Chains      []string       `json:"chains,omitempty"`
	Rule        string         `json:"rule,omitempty"`
	RulePayload string         `json:"rule_payload,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ProvidersSnapshot struct {
	ProxyProviders []ProxyProvider `json:"proxy_providers"`
	RuleProviders  []RuleProvider  `json:"rule_providers"`
}

type ProxyProvider struct {
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	VehicleType    string          `json:"vehicle_type"`
	UpdatedAt      string          `json:"updated_at,omitempty"`
	TestURL        string          `json:"test_url,omitempty"`
	ExpectedStatus string          `json:"expected_status,omitempty"`
	ProxyCount     int             `json:"proxy_count"`
	Proxies        []ProviderProxy `json:"proxies"`
}

type ProviderProxy struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Alive bool   `json:"alive"`
}

type RuleProvider struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	VehicleType string `json:"vehicle_type"`
	Behavior    string `json:"behavior,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	RuleCount   int    `json:"rule_count"`
}

type proxyRecord struct {
	Name     string               `json:"name"`
	Type     string               `json:"type"`
	Now      string               `json:"now"`
	All      []string             `json:"all"`
	Provider string               `json:"provider"`
	UDP      bool                 `json:"udp"`
	Alive    *bool                `json:"alive"`
	History  []proxyHistoryRecord `json:"history"`
}

type proxyHistoryRecord struct {
	Time  string `json:"time"`
	Delay int    `json:"delay"`
}

type proxiesResponse struct {
	Proxies map[string]proxyRecord `json:"proxies"`
}

type connectionsResponse struct {
	UploadTotal   int64              `json:"uploadTotal"`
	DownloadTotal int64              `json:"downloadTotal"`
	Connections   []connectionRecord `json:"connections"`
}

type proxyProvidersResponse struct {
	Providers map[string]proxyProviderRecord `json:"providers"`
}

type proxyProviderRecord struct {
	Name           string                `json:"name"`
	Type           string                `json:"type"`
	VehicleType    string                `json:"vehicleType"`
	UpdatedAt      string                `json:"updatedAt"`
	TestURL        string                `json:"testUrl"`
	ExpectedStatus string                `json:"expectedStatus"`
	Proxies        []providerProxyRecord `json:"proxies"`
}

type providerProxyRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Alive bool   `json:"alive"`
}

type ruleProvidersResponse struct {
	Providers map[string]ruleProviderRecord `json:"providers"`
}

type ruleProviderRecord struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	VehicleType string `json:"vehicleType"`
	Behavior    string `json:"behavior"`
	UpdatedAt   string `json:"updatedAt"`
	RuleCount   int    `json:"ruleCount"`
	Rules       []any  `json:"rules"`
}

type connectionRecord struct {
	ID          string         `json:"id"`
	Upload      int64          `json:"upload"`
	Download    int64          `json:"download"`
	Start       string         `json:"start"`
	Chains      []string       `json:"chains"`
	Rule        string         `json:"rule"`
	RulePayload string         `json:"rulePayload"`
	Metadata    map[string]any `json:"metadata"`
}

func FetchVersion(ctx context.Context, cfg config.Config) (Version, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	return fetchVersionWithClient(ctx, cfg, client)
}

func FetchProxyGroups(ctx context.Context, cfg config.Config) ([]ProxyGroup, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	return fetchProxyGroupsWithClient(ctx, cfg, client)
}

func FetchProxyHealth(ctx context.Context, cfg config.Config) (ProxyHealthSnapshot, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	return fetchProxyHealthWithClient(ctx, cfg, client)
}

func MeasureProxyDelay(ctx context.Context, cfg config.Config, proxyName, testURL string, timeout time.Duration) ProxyDelayResult {
	if strings.TrimSpace(testURL) == "" {
		testURL = DefaultProxyDelayTestURL
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := &http.Client{Timeout: timeout + time.Second}
	return measureProxyDelayWithClient(ctx, cfg, client, proxyName, testURL, timeout)
}

func SelectProxyGroup(ctx context.Context, cfg config.Config, groupName, selected string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	return selectProxyGroupWithClient(ctx, cfg, client, groupName, selected)
}

func FetchConnections(ctx context.Context, cfg config.Config) (ConnectionsSnapshot, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	return fetchConnectionsWithClient(ctx, cfg, client)
}

func FetchProviders(ctx context.Context, cfg config.Config) (ProvidersSnapshot, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	return fetchProvidersWithClient(ctx, cfg, client)
}

func UpdateProxyProvider(ctx context.Context, cfg config.Config, providerName string) (ProxyProvider, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	return updateProxyProviderWithClient(ctx, cfg, client, providerName)
}

func fetchVersionWithClient(ctx context.Context, cfg config.Config, client *http.Client) (Version, error) {
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, "/version", nil)
	if err != nil {
		return Version{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return Version{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Version{}, fmt.Errorf("mihomo API returned %s", resp.Status)
	}

	var version Version
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		if errors.Is(err, io.EOF) {
			return Version{}, fmt.Errorf("empty mihomo API response")
		}
		return Version{}, err
	}
	return version, nil
}

func fetchProxyGroupsWithClient(ctx context.Context, cfg config.Config, client *http.Client) ([]ProxyGroup, error) {
	body, err := fetchProxiesWithClient(ctx, cfg, client)
	if err != nil {
		return nil, err
	}

	groups := make([]ProxyGroup, 0, len(body.Proxies))
	for name, proxy := range body.Proxies {
		if len(proxy.All) == 0 {
			continue
		}
		if proxy.Name == "" {
			proxy.Name = name
		}
		groups = append(groups, ProxyGroup{
			Name:     proxy.Name,
			Type:     proxy.Type,
			Selected: proxy.Now,
			Options:  proxy.All,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return groups, nil
}

func fetchProxyHealthWithClient(ctx context.Context, cfg config.Config, client *http.Client) (ProxyHealthSnapshot, error) {
	body, err := fetchProxiesWithClient(ctx, cfg, client)
	if err != nil {
		return ProxyHealthSnapshot{}, err
	}

	proxies := make([]ProxyHealth, 0, len(body.Proxies))
	for name, proxy := range body.Proxies {
		if proxy.Name == "" {
			proxy.Name = name
		}
		health := ProxyHealth{
			Name:      proxy.Name,
			Type:      proxy.Type,
			Selected:  proxy.Now,
			Provider:  proxy.Provider,
			UDP:       proxy.UDP,
			Status:    "untested",
			Probeable: proxyIsProbeable(proxy.Type),
		}
		if !health.Probeable {
			health.Status = "not_applicable"
		}
		if health.Probeable && len(proxy.History) > 0 {
			latest := proxy.History[len(proxy.History)-1]
			health.DelayMS = latest.Delay
			health.TestedAt = latest.Time
			if latest.Delay > 0 {
				health.Status = "reachable"
			} else if health.Probeable {
				health.Status = "unreachable"
			}
		} else if health.Probeable && proxy.Alive != nil {
			if *proxy.Alive {
				health.Status = "reachable"
			} else if health.Probeable {
				health.Status = "unreachable"
			}
		}
		proxies = append(proxies, health)
	}
	sort.Slice(proxies, func(i, j int) bool { return proxies[i].Name < proxies[j].Name })
	return ProxyHealthSnapshot{TestURL: DefaultProxyDelayTestURL, Proxies: proxies}, nil
}

func fetchProxiesWithClient(ctx context.Context, cfg config.Config, client *http.Client) (proxiesResponse, error) {
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, "/proxies", nil)
	if err != nil {
		return proxiesResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return proxiesResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return proxiesResponse{}, fmt.Errorf("mihomo API returned %s", resp.Status)
	}
	var body proxiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return proxiesResponse{}, fmt.Errorf("empty mihomo API response")
		}
		return proxiesResponse{}, err
	}
	return body, nil
}

func measureProxyDelayWithClient(ctx context.Context, cfg config.Config, client *http.Client, proxyName, testURL string, timeout time.Duration) ProxyDelayResult {
	result := ProxyDelayResult{Name: proxyName, Status: "unreachable", TestedAt: time.Now().UTC().Format(time.RFC3339), TestURL: testURL}
	query := url.Values{}
	query.Set("url", testURL)
	query.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	path := "/proxies/" + url.PathEscape(proxyName) + "/delay?" + query.Encode()
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, path, nil)
	if err != nil {
		result.Status, result.Error = "error", err.Error()
		return result
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			result.Status = "timeout"
		} else if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
			result.Status = "timeout"
		}
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		result.Error = strings.TrimSpace(string(message))
		if resp.StatusCode == http.StatusGatewayTimeout || strings.Contains(strings.ToLower(result.Error), "timeout") || strings.Contains(strings.ToLower(result.Error), "deadline exceeded") {
			result.Status = "timeout"
		}
		if result.Error == "" {
			result.Error = "mihomo API returned " + resp.Status
		}
		return result
	}
	var body struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		result.Status, result.Error = "error", err.Error()
		return result
	}
	if body.Delay <= 0 {
		result.Error = "proxy delay probe returned no usable delay"
		return result
	}
	result.Status = "reachable"
	result.DelayMS = body.Delay
	return result
}

func proxyIsProbeable(proxyType string) bool {
	switch strings.ToLower(strings.TrimSpace(proxyType)) {
	case "reject", "rejectdrop", "reject-drop", "pass", "pass-rule", "compatible":
		return false
	default:
		return true
	}
}

func fetchConnectionsWithClient(ctx context.Context, cfg config.Config, client *http.Client) (ConnectionsSnapshot, error) {
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, "/connections", nil)
	if err != nil {
		return ConnectionsSnapshot{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return ConnectionsSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ConnectionsSnapshot{}, fmt.Errorf("mihomo API returned %s", resp.Status)
	}

	var body connectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return ConnectionsSnapshot{}, fmt.Errorf("empty mihomo API response")
		}
		return ConnectionsSnapshot{}, err
	}

	connections := make([]Connection, 0, len(body.Connections))
	for _, connection := range body.Connections {
		connections = append(connections, Connection{
			ID:          connection.ID,
			Upload:      connection.Upload,
			Download:    connection.Download,
			Start:       connection.Start,
			Chains:      connection.Chains,
			Rule:        connection.Rule,
			RulePayload: connection.RulePayload,
			Metadata:    connection.Metadata,
		})
	}

	return ConnectionsSnapshot{
		UploadTotal:   body.UploadTotal,
		DownloadTotal: body.DownloadTotal,
		Connections:   connections,
	}, nil
}

func fetchProvidersWithClient(ctx context.Context, cfg config.Config, client *http.Client) (ProvidersSnapshot, error) {
	proxyProviders, err := fetchProxyProvidersWithClient(ctx, cfg, client)
	if err != nil {
		return ProvidersSnapshot{}, err
	}
	ruleProviders, err := fetchRuleProvidersWithClient(ctx, cfg, client)
	if err != nil {
		return ProvidersSnapshot{}, err
	}
	return ProvidersSnapshot{
		ProxyProviders: proxyProviders,
		RuleProviders:  ruleProviders,
	}, nil
}

func fetchProxyProvidersWithClient(ctx context.Context, cfg config.Config, client *http.Client) ([]ProxyProvider, error) {
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, "/providers/proxies", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mihomo API returned %s", resp.Status)
	}

	var body proxyProvidersResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("empty mihomo API response")
		}
		return nil, err
	}

	providers := make([]ProxyProvider, 0, len(body.Providers))
	for name, provider := range body.Providers {
		if provider.Name == "" {
			provider.Name = name
		}
		proxies := make([]ProviderProxy, 0, len(provider.Proxies))
		for _, proxy := range provider.Proxies {
			proxies = append(proxies, ProviderProxy{
				Name:  proxy.Name,
				Type:  proxy.Type,
				Alive: proxy.Alive,
			})
		}
		providers = append(providers, ProxyProvider{
			Name:           provider.Name,
			Type:           provider.Type,
			VehicleType:    provider.VehicleType,
			UpdatedAt:      provider.UpdatedAt,
			TestURL:        provider.TestURL,
			ExpectedStatus: provider.ExpectedStatus,
			ProxyCount:     len(proxies),
			Proxies:        proxies,
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	return providers, nil
}

func updateProxyProviderWithClient(ctx context.Context, cfg config.Config, client *http.Client, providerName string) (ProxyProvider, error) {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return ProxyProvider{}, fmt.Errorf("proxy provider is required")
	}

	req, err := newAPIRequest(ctx, cfg, http.MethodPut, "/providers/proxies/"+url.PathEscape(providerName), nil)
	if err != nil {
		return ProxyProvider{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProxyProvider{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProxyProvider{}, fmt.Errorf("mihomo API returned %s", resp.Status)
	}

	providers, err := fetchProxyProvidersWithClient(ctx, cfg, client)
	if err != nil {
		return ProxyProvider{}, err
	}
	provider, ok := findProxyProvider(providers, providerName)
	if !ok {
		return ProxyProvider{}, fmt.Errorf("proxy provider %q not found after update", providerName)
	}
	return provider, nil
}

func findProxyProvider(providers []ProxyProvider, providerName string) (ProxyProvider, bool) {
	for _, provider := range providers {
		if provider.Name == providerName {
			return provider, true
		}
	}
	return ProxyProvider{}, false
}

func fetchRuleProvidersWithClient(ctx context.Context, cfg config.Config, client *http.Client) ([]RuleProvider, error) {
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, "/providers/rules", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mihomo API returned %s", resp.Status)
	}

	var body ruleProvidersResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("empty mihomo API response")
		}
		return nil, err
	}

	providers := make([]RuleProvider, 0, len(body.Providers))
	for name, provider := range body.Providers {
		if provider.Name == "" {
			provider.Name = name
		}
		ruleCount := provider.RuleCount
		if ruleCount == 0 && len(provider.Rules) > 0 {
			ruleCount = len(provider.Rules)
		}
		providers = append(providers, RuleProvider{
			Name:        provider.Name,
			Type:        provider.Type,
			VehicleType: provider.VehicleType,
			Behavior:    provider.Behavior,
			UpdatedAt:   provider.UpdatedAt,
			RuleCount:   ruleCount,
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	return providers, nil
}

func selectProxyGroupWithClient(ctx context.Context, cfg config.Config, client *http.Client, groupName, selected string) error {
	if strings.TrimSpace(groupName) == "" {
		return fmt.Errorf("policy group is required")
	}
	if strings.TrimSpace(selected) == "" {
		return fmt.Errorf("selected policy is required")
	}
	body, err := json.Marshal(map[string]string{"name": selected})
	if err != nil {
		return err
	}
	path := "/proxies/" + url.PathEscape(groupName)
	req, err := newAPIRequest(ctx, cfg, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mihomo API returned %s", resp.Status)
	}
	return nil
}

func newAPIRequest(ctx context.Context, cfg config.Config, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, apiURL(cfg, path), body)
	if err != nil {
		return nil, err
	}
	if cfg.Mihomo.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Mihomo.Secret)
	}
	return req, nil
}

func apiURL(cfg config.Config, path string) string {
	base := cfg.Mihomo.APIAddr
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	return strings.TrimRight(base, "/") + path
}
