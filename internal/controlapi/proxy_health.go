package controlapi

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/mihomo"
)

const proxyHealthConcurrency = 6

type ProxyHealthResponse struct {
	SchemaVersion int                  `json:"schema_version"`
	TestURL       string               `json:"test_url"`
	Proxies       []mihomo.ProxyHealth `json:"proxies"`
}

type ProxyHealthTestRequest struct {
	Names []string `json:"names"`
}

type ProxyHealthTestResponse struct {
	SchemaVersion int                       `json:"schema_version"`
	TestURL       string                    `json:"test_url"`
	Results       []mihomo.ProxyDelayResult `json:"results"`
}

func (s *Server) handleProxyHealth(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	snapshot, err := s.fetchProxyHealth(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "mihomo_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ProxyHealthResponse{SchemaVersion: SchemaVersion, TestURL: snapshot.TestURL, Proxies: snapshot.Proxies})
}

func (s *Server) handleProxyHealthTests(w http.ResponseWriter, r *http.Request) {
	var request ProxyHealthTestRequest
	if err := decodeJSON(r, &request, 128<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	names := uniqueProxyNames(request.Names)
	if len(names) == 0 || len(names) > 128 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_proxy_names", "choose between 1 and 128 current proxies")
		return
	}
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	snapshot, err := s.fetchProxyHealth(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "mihomo_unavailable", err.Error())
		return
	}
	available := make(map[string]mihomo.ProxyHealth, len(snapshot.Proxies))
	for _, proxy := range snapshot.Proxies {
		available[proxy.Name] = proxy
	}
	for _, name := range names {
		proxy, exists := available[name]
		if !exists || !proxy.Probeable {
			writeError(w, http.StatusUnprocessableEntity, "invalid_proxy_names", "one or more proxies are unavailable or cannot be tested")
			return
		}
	}

	results := make([]mihomo.ProxyDelayResult, len(names))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(proxyHealthConcurrency, len(names)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				results[index] = s.measureProxyDelay(r.Context(), cfg, names[index], snapshot.TestURL, 5*time.Second)
			}
		}()
	}
	for index := range names {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	writeJSON(w, http.StatusOK, ProxyHealthTestResponse{SchemaVersion: SchemaVersion, TestURL: snapshot.TestURL, Results: results})
}

func uniqueProxyNames(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	names := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
