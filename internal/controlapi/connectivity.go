package controlapi

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/mihomo"
)

const (
	connectivityProbeRounds      = 3
	connectivityProbeConcurrency = 4
	connectivityProbeTimeout     = 4 * time.Second
)

type ConnectivityTarget struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Symbol        string `json:"symbol"`
	URL           string `json:"url"`
	ExpectedRoute string `json:"expected_route"`
}

type ConnectivitySample struct {
	Status     string `json:"status"`
	DelayMS    int    `json:"delay_ms,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Error      string `json:"error,omitempty"`
}

type ConnectivityResult struct {
	TargetID    string               `json:"target_id"`
	Status      string               `json:"status"`
	Grade       string               `json:"grade"`
	MedianMS    int                  `json:"median_ms,omitempty"`
	HTTPStatus  int                  `json:"http_status,omitempty"`
	Chain       []string             `json:"chain"`
	Rule        string               `json:"rule,omitempty"`
	RulePayload string               `json:"rule_payload,omitempty"`
	Route       string               `json:"route"`
	RouteMatch  *bool                `json:"route_match,omitempty"`
	Samples     []ConnectivitySample `json:"samples"`
	TestedAt    time.Time            `json:"tested_at"`
}

type ConnectivityResponse struct {
	SchemaVersion int                  `json:"schema_version"`
	Source        string               `json:"source"`
	Scope         string               `json:"scope"`
	Rounds        int                  `json:"rounds"`
	Targets       []ConnectivityTarget `json:"targets"`
	Results       []ConnectivityResult `json:"results"`
	StartedAt     *time.Time           `json:"started_at,omitempty"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty"`
}

type ConnectivityTestRequest struct {
	TargetIDs []string `json:"target_ids"`
}

var defaultConnectivityTargets = []ConnectivityTarget{
	{ID: "deepseek", Name: "DeepSeek", Category: "china", Symbol: "DS", URL: "https://www.deepseek.com/favicon.ico", ExpectedRoute: "direct"},
	{ID: "bilibili", Name: "哔哩哔哩", Category: "china", Symbol: "哔", URL: "https://www.bilibili.com/favicon.ico", ExpectedRoute: "direct"},
	{ID: "baidu", Name: "百度", Category: "china", Symbol: "百", URL: "https://www.baidu.com/favicon.ico", ExpectedRoute: "direct"},
	{ID: "jd", Name: "京东", Category: "china", Symbol: "JD", URL: "https://www.jd.com/favicon.ico", ExpectedRoute: "direct"},
	{ID: "apple", Name: "Apple", Category: "global", Symbol: "A", URL: "https://www.apple.com/favicon.ico", ExpectedRoute: "proxy"},
	{ID: "google", Name: "Google", Category: "global", Symbol: "G", URL: "https://www.google.com/generate_204", ExpectedRoute: "proxy"},
	{ID: "youtube", Name: "YouTube", Category: "global", Symbol: "YT", URL: "https://www.youtube.com/favicon.ico", ExpectedRoute: "proxy"},
	{ID: "wikipedia", Name: "Wikipedia", Category: "global", Symbol: "W", URL: "https://www.wikipedia.org/favicon.ico", ExpectedRoute: "proxy"},
	{ID: "chatgpt", Name: "ChatGPT", Category: "ai", Symbol: "AI", URL: "https://chatgpt.com/favicon.ico", ExpectedRoute: "proxy"},
	{ID: "claude", Name: "Claude", Category: "ai", Symbol: "C", URL: "https://claude.ai/favicon.ico", ExpectedRoute: "proxy"},
	{ID: "gemini", Name: "Gemini", Category: "ai", Symbol: "Gm", URL: "https://gemini.google.com/favicon.ico", ExpectedRoute: "proxy"},
	{ID: "github", Name: "GitHub", Category: "developer", Symbol: "GH", URL: "https://github.com/favicon.ico", ExpectedRoute: "proxy"},
	{ID: "npm", Name: "npm", Category: "developer", Symbol: "npm", URL: "https://registry.npmjs.org/-/ping", ExpectedRoute: "proxy"},
	{ID: "cloudflare", Name: "Cloudflare", Category: "developer", Symbol: "CF", URL: "https://cp.cloudflare.com/generate_204", ExpectedRoute: "proxy"},
}

func (s *Server) handleConnectivity(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ConnectivityResponse{
		SchemaVersion: SchemaVersion,
		Source:        "gateway_mihomo",
		Scope:         "applied_global_rules",
		Rounds:        connectivityProbeRounds,
		Targets:       connectivityCatalog(),
		Results:       []ConnectivityResult{},
	})
}

func (s *Server) handleConnectivityTests(w http.ResponseWriter, r *http.Request) {
	var request ConnectivityTestRequest
	if err := decodeJSON(r, &request, 128<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	targets, err := selectedConnectivityTargets(request.TargetIDs)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_connectivity_targets", err.Error())
		return
	}
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	startedAt := time.Now().UTC()
	results := make([]ConnectivityResult, len(targets))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(connectivityProbeConcurrency, len(targets)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				results[index] = s.probeConnectivity(r.Context(), cfg, targets[index])
			}
		}()
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	completedAt := time.Now().UTC()
	writeJSON(w, http.StatusOK, ConnectivityResponse{
		SchemaVersion: SchemaVersion,
		Source:        "gateway_mihomo",
		Scope:         "applied_global_rules",
		Rounds:        connectivityProbeRounds,
		Targets:       connectivityCatalog(),
		Results:       results,
		StartedAt:     &startedAt,
		CompletedAt:   &completedAt,
	})
}

func connectivityCatalog() []ConnectivityTarget {
	return append([]ConnectivityTarget(nil), defaultConnectivityTargets...)
}

func selectedConnectivityTargets(ids []string) ([]ConnectivityTarget, error) {
	if len(ids) == 0 {
		return connectivityCatalog(), nil
	}
	if len(ids) > len(defaultConnectivityTargets) {
		return nil, errors.New("too many connectivity targets")
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	targets := make([]ConnectivityTarget, 0, len(wanted))
	for _, target := range defaultConnectivityTargets {
		if _, exists := wanted[target.ID]; exists {
			targets = append(targets, target)
			delete(wanted, target.ID)
		}
	}
	if len(wanted) > 0 || len(targets) == 0 {
		return nil, errors.New("one or more connectivity targets are unknown")
	}
	return targets, nil
}

func probeConnectivityTarget(ctx context.Context, cfg config.Config, target ConnectivityTarget) ConnectivityResult {
	result := ConnectivityResult{TargetID: target.ID, Status: "unreachable", Grade: "timeout", Route: "unknown", Chain: []string{}, Samples: make([]ConnectivitySample, 0, connectivityProbeRounds), TestedAt: time.Now().UTC()}
	delays := make([]int, 0, connectivityProbeRounds)
	for range connectivityProbeRounds {
		sample, connection := probeConnectivityRound(ctx, cfg, target)
		result.Samples = append(result.Samples, sample)
		if sample.Status == "reachable" {
			delays = append(delays, sample.DelayMS)
			result.HTTPStatus = sample.HTTPStatus
		}
		if connection != nil && len(result.Chain) == 0 {
			result.Chain = append([]string(nil), connection.Chains...)
			result.Rule = connection.Rule
			result.RulePayload = connection.RulePayload
		}
		if ctx.Err() != nil {
			break
		}
	}
	if len(delays) > 0 {
		sort.Ints(delays)
		result.MedianMS = delays[len(delays)/2]
		result.Status = "reachable"
		if len(delays) < connectivityProbeRounds {
			result.Status = "degraded"
		}
		result.Grade = connectivityGrade(result.MedianMS)
	} else if len(result.Samples) > 0 {
		result.Status = result.Samples[len(result.Samples)-1].Status
	}
	result.Route = connectivityRoute(result.Chain)
	if result.Route != "unknown" && target.ExpectedRoute != "any" {
		matches := result.Route == target.ExpectedRoute
		result.RouteMatch = &matches
	}
	result.TestedAt = time.Now().UTC()
	return result
}

func probeConnectivityRound(ctx context.Context, cfg config.Config, target ConnectivityTarget) (ConnectivitySample, *mihomo.Connection) {
	roundCtx, cancel := context.WithTimeout(ctx, connectivityProbeTimeout)
	defer cancel()
	parsed, err := url.Parse(target.URL)
	if err != nil {
		return ConnectivitySample{Status: "configuration_error", Error: err.Error()}, nil
	}
	before := currentConnectionIDs(roundCtx, cfg)
	proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Mihomo.MixedPort))}
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		DialContext:         (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: -1}).DialContext,
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: 3 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   connectivityProbeTimeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(roundCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		return ConnectivitySample{Status: "configuration_error", Error: err.Error()}, nil
	}
	req.Header.Set("User-Agent", "OpenSurge/0.1 connectivity-probe")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Range", "bytes=0-1023")
	started := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		return ConnectivitySample{Status: connectivityErrorStatus(err), Error: err.Error()}, nil
	}
	finalHost := parsed.Hostname()
	if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.Hostname() != "" {
		finalHost = resp.Request.URL.Hostname()
	}
	connection := captureProbeConnection(roundCtx, cfg, before, finalHost)
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	return ConnectivitySample{Status: "reachable", DelayMS: max(latency, 1), HTTPStatus: resp.StatusCode}, connection
}

func currentConnectionIDs(ctx context.Context, cfg config.Config) map[string]struct{} {
	probeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	snapshot, err := mihomo.FetchConnections(probeCtx, cfg)
	if err != nil {
		return map[string]struct{}{}
	}
	ids := make(map[string]struct{}, len(snapshot.Connections))
	for _, connection := range snapshot.Connections {
		ids[connection.ID] = struct{}{}
	}
	return ids
}

func captureProbeConnection(ctx context.Context, cfg config.Config, before map[string]struct{}, host string) *mihomo.Connection {
	deadline := time.Now().Add(450 * time.Millisecond)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		probeCtx, cancel := context.WithTimeout(ctx, 180*time.Millisecond)
		snapshot, err := mihomo.FetchConnections(probeCtx, cfg)
		cancel()
		if err == nil {
			for index := range snapshot.Connections {
				connection := &snapshot.Connections[index]
				if _, existed := before[connection.ID]; existed {
					continue
				}
				if sameConnectionHost(connection.Metadata, host) {
					copy := *connection
					return &copy
				}
			}
		}
		timer := time.NewTimer(35 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

func sameConnectionHost(metadata map[string]any, host string) bool {
	value, _ := metadata["host"].(string)
	value = strings.TrimSpace(value)
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		value = parsedHost
	}
	return strings.EqualFold(strings.Trim(value, "[]"), host)
}

func connectivityErrorStatus(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns_error"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "tls") || strings.Contains(message, "x509") || strings.Contains(message, "certificate") {
		return "tls_error"
	}
	return "connection_error"
}

func connectivityGrade(delay int) string {
	switch {
	case delay <= 200:
		return "excellent"
	case delay <= 600:
		return "good"
	case delay <= 1500:
		return "slow"
	default:
		return "very_slow"
	}
}

func connectivityRoute(chain []string) string {
	if len(chain) == 0 {
		return "unknown"
	}
	for _, item := range chain {
		switch strings.ToUpper(strings.TrimSpace(item)) {
		case "DIRECT":
			return "direct"
		case "REJECT", "REJECT-DROP":
			return "reject"
		}
	}
	return "proxy"
}
