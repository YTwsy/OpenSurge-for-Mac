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

type proxyRecord struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now"`
	All  []string `json:"all"`
}

type proxiesResponse struct {
	Proxies map[string]proxyRecord `json:"proxies"`
}

type connectionsResponse struct {
	UploadTotal   int64              `json:"uploadTotal"`
	DownloadTotal int64              `json:"downloadTotal"`
	Connections   []connectionRecord `json:"connections"`
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

func SelectProxyGroup(ctx context.Context, cfg config.Config, groupName, selected string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	return selectProxyGroupWithClient(ctx, cfg, client, groupName, selected)
}

func FetchConnections(ctx context.Context, cfg config.Config) (ConnectionsSnapshot, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	return fetchConnectionsWithClient(ctx, cfg, client)
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
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, "/proxies", nil)
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

	var body proxiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("empty mihomo API response")
		}
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
