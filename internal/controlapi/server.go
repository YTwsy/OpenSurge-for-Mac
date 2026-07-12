package controlapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/doctor"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

type Options struct {
	ConfigPath      string
	Addr            string
	StoreDir        string
	Runner          ActionRunner
	NetworkRunner   NetworkRunner
	ConfigRunner    ConfigurationRunner
	DiscoverNetwork func(context.Context, string, string) (macosnetwork.Snapshot, error)
	PingRouter      func(context.Context, string) error
	Static          http.Handler
	Credentials     SourceCredentialStore
}

type Server struct {
	configPath       string
	addr             string
	store            *Store
	runner           ActionRunner
	networkRunner    NetworkRunner
	configRunner     ConfigurationRunner
	discoverNetwork  func(context.Context, string, string) (macosnetwork.Snapshot, error)
	pingRouter       func(context.Context, string) error
	static           http.Handler
	credentials      SourceCredentialStore
	fetchConnections func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error)
	token            string
	baseURL          string

	mu         sync.Mutex
	sessions   map[string]time.Time
	bootstraps map[string]bootstrapGrant
}

type bootstrapGrant struct {
	expires time.Time
	path    string
}

func New(options Options) (*Server, error) {
	if options.ConfigPath == "" {
		return nil, fmt.Errorf("config path is required")
	}
	configPath, err := filepath.Abs(options.ConfigPath)
	if err != nil {
		return nil, err
	}
	if options.Addr == "" {
		options.Addr = "127.0.0.1:61767"
	}
	host, _, err := net.SplitHostPort(options.Addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost") {
		return nil, fmt.Errorf("control API must listen on loopback IPv4")
	}
	if options.StoreDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		options.StoreDir = filepath.Join(home, "Library", "Application Support", "OpenSurge")
	}
	store := NewStore(options.StoreDir)
	if err := store.Ensure(); err != nil {
		return nil, err
	}
	token, err := store.Token()
	if err != nil {
		return nil, err
	}
	if options.Runner == nil {
		options.Runner = HelperClient{SocketPath: "/var/run/opensurge/helper.sock"}
	}
	if options.NetworkRunner == nil {
		if runner, ok := options.Runner.(NetworkRunner); ok {
			options.NetworkRunner = runner
		} else {
			options.NetworkRunner = HelperClient{SocketPath: "/var/run/opensurge/helper.sock"}
		}
	}
	if options.ConfigRunner == nil {
		if runner, ok := options.Runner.(ConfigurationRunner); ok {
			options.ConfigRunner = runner
		} else {
			options.ConfigRunner = HelperClient{SocketPath: "/var/run/opensurge/helper.sock"}
		}
	}
	if options.DiscoverNetwork == nil {
		options.DiscoverNetwork = macosnetwork.Discover
	}
	if options.PingRouter == nil {
		options.PingRouter = macosnetwork.PingRouter
	}
	if options.Credentials == nil {
		options.Credentials = KeychainCredentialStore{}
	}
	if err := migrateSourceCredentials(context.Background(), store, options.Credentials); err != nil {
		return nil, err
	}
	return &Server{
		configPath:       configPath,
		addr:             options.Addr,
		store:            store,
		runner:           options.Runner,
		networkRunner:    options.NetworkRunner,
		configRunner:     options.ConfigRunner,
		discoverNetwork:  options.DiscoverNetwork,
		pingRouter:       options.PingRouter,
		static:           options.Static,
		credentials:      options.Credentials,
		fetchConnections: mihomo.FetchConnections,
		token:            token,
		baseURL:          "http://" + options.Addr,
		sessions:         map[string]time.Time{},
		bootstraps:       map[string]bootstrapGrant{},
	}, nil
}

func (s *Server) BootstrapURL() string {
	return s.bootstrapURLFor("dashboard")
}

func (s *Server) bootstrapURLFor(path string) string {
	path = allowedWebPath(path)
	code := randomToken(24)
	s.mu.Lock()
	s.bootstraps[code] = bootstrapGrant{expires: time.Now().Add(30 * time.Second), path: path}
	s.mu.Unlock()
	return s.baseURL + "/bootstrap?code=" + code
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /bootstrap", s.exchangeBootstrap)
	mux.HandleFunc("POST /api/v1/session/bootstrap", s.handleSessionBootstrap)
	mux.Handle("GET /api/v1/overview", s.auth(http.HandlerFunc(s.handleOverview)))
	mux.Handle("GET /api/v1/config", s.auth(http.HandlerFunc(s.handleControlConfig)))
	mux.Handle("PUT /api/v1/config", s.auth(http.HandlerFunc(s.handleControlConfig)))
	mux.Handle("GET /api/v1/menubar", s.auth(http.HandlerFunc(s.handleMenuBar)))
	mux.Handle("GET /api/v1/gateway/plan", s.auth(http.HandlerFunc(s.handleGatewayPlan)))
	mux.Handle("POST /api/v1/gateway/plan", s.auth(http.HandlerFunc(s.handleGatewayPlan)))
	mux.Handle("POST /api/v1/gateway/start", s.auth(http.HandlerFunc(s.handleGatewayAction)))
	mux.Handle("POST /api/v1/gateway/stop", s.auth(http.HandlerFunc(s.handleGatewayAction)))
	mux.Handle("GET /api/v1/recovery", s.auth(http.HandlerFunc(s.handleRecovery)))
	mux.Handle("POST /api/v1/recovery", s.auth(http.HandlerFunc(s.handleRecovery)))
	mux.Handle("GET /api/v1/recovery/card", s.auth(http.HandlerFunc(s.handleRecoveryCard)))
	mux.Handle("POST /api/v1/recovery/discard", s.auth(http.HandlerFunc(s.handleRecoveryDiscard)))
	mux.Handle("POST /api/v1/recovery/prepare", s.auth(http.HandlerFunc(s.handleRecoveryPrepare)))
	mux.Handle("POST /api/v1/recovery/router-restored", s.auth(http.HandlerFunc(s.handleRouterRestored)))
	mux.Handle("POST /api/v1/recovery/client-validated", s.auth(http.HandlerFunc(s.handleClientValidated)))
	mux.Handle("GET /api/v1/network/discovery", s.auth(http.HandlerFunc(s.handleNetworkDiscovery)))
	mux.Handle("POST /api/v1/network/apply-static", s.auth(http.HandlerFunc(s.handleApplyStatic)))
	mux.Handle("POST /api/v1/network/dhcp-probe", s.auth(http.HandlerFunc(s.handleDHCPProbe)))
	mux.Handle("POST /api/v1/network/restore-dhcp", s.auth(http.HandlerFunc(s.handleRestoreDHCP)))
	mux.Handle("GET /api/v1/sources", s.auth(http.HandlerFunc(s.handleSources)))
	mux.Handle("POST /api/v1/sources", s.auth(http.HandlerFunc(s.handleSources)))
	mux.Handle("POST /api/v1/sources/{id}/refresh", s.auth(http.HandlerFunc(s.handleSourceRefresh)))
	mux.Handle("POST /api/v1/sources/{id}/apply", s.auth(http.HandlerFunc(s.handleSourceApply)))
	mux.Handle("GET /api/v1/device-policy", s.auth(http.HandlerFunc(s.handleDevicePolicy)))
	mux.Handle("PUT /api/v1/device-policy", s.auth(http.HandlerFunc(s.handleDevicePolicy)))
	mux.Handle("GET /api/v1/devices", s.auth(http.HandlerFunc(s.handleDevices)))
	mux.Handle("GET /api/v1/device-traffic", s.auth(http.HandlerFunc(s.handleDeviceTraffic)))
	mux.Handle("POST /api/v1/devices/{device}/selectors/{slot}", s.auth(http.HandlerFunc(s.handleDeviceSelection)))
	mux.Handle("GET /api/v1/policies", s.auth(http.HandlerFunc(s.handlePolicies)))
	mux.Handle("POST /api/v1/policies/{group}/selection", s.auth(http.HandlerFunc(s.handlePolicySelection)))
	mux.Handle("GET /api/v1/providers", s.auth(http.HandlerFunc(s.handleProviders)))
	mux.Handle("GET /api/v1/diagnostics", s.auth(http.HandlerFunc(s.handleDiagnostics)))
	mux.Handle("POST /api/v1/providers/{name}/refresh", s.auth(http.HandlerFunc(s.handleProviderRefresh)))
	mux.Handle("GET /api/v1/operations/{id}", s.auth(http.HandlerFunc(s.handleOperation)))
	mux.Handle("GET /api/v1/operations", s.auth(http.HandlerFunc(s.handleOperations)))
	mux.Handle("GET /api/v1/events", s.auth(http.HandlerFunc(s.handleEvents)))
	if s.static != nil {
		mux.Handle("/", s.static)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Web UI is not built", http.StatusServiceUnavailable)
		})
	}
	return s.securityHeaders(mux)
}

func (s *Server) handleControlConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	revision := fileDigest(s.configPath)
	if r.Method == http.MethodGet {
		w.Header().Set("ETag", `"`+revision+`"`)
		writeJSON(w, http.StatusOK, controlConfigFrom(cfg, revision))
		return
	}
	recovery, _ := s.store.Recovery()
	if recovery.Required && recovery.Stage != RecoveryPrepared {
		writeError(w, http.StatusConflict, "recovery_required", "finish network recovery before editing topology")
		return
	}
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != revision {
		writeError(w, http.StatusConflict, "revision_conflict", "If-Match must contain the current config revision")
		return
	}
	var input ControlConfig
	if err := decodeJSON(r, &input, 256<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	payload, _ := json.Marshal(input)
	newRevision, err := s.configRunner.ApplyControlConfig(r.Context(), s.configPath, match, payload)
	if err != nil {
		status, code := http.StatusUnprocessableEntity, "config_validation_failed"
		if strings.Contains(err.Error(), "revision conflict") {
			status, code = http.StatusConflict, "revision_conflict"
		}
		if strings.Contains(err.Error(), "must be stopped") {
			status, code = http.StatusConflict, "gateway_running"
		}
		writeError(w, status, code, err.Error())
		return
	}
	cfg, err = config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_reload_failed", err.Error())
		return
	}
	if recovery.Stage == RecoveryPrepared {
		if err := s.store.DiscardPreparedRecovery(cfg.Gateway.Mode); err != nil {
			writeError(w, http.StatusInternalServerError, "recovery_discard_failed", "configuration was saved but the prepared recovery card could not be discarded: "+err.Error())
			return
		}
	} else if err := s.store.SaveRecovery(RecoveryState{SchemaVersion: SchemaVersion, Stage: RecoveryIdle, Topology: cfg.Gateway.Mode}); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	w.Header().Set("ETag", `"`+newRevision+`"`)
	writeJSON(w, http.StatusOK, controlConfigFrom(cfg, newRevision))
}

func controlConfigFrom(cfg config.Config, revision string) ControlConfig {
	dnsUpstream := strings.TrimSpace(cfg.DNS.Upstream)
	if dnsUpstream == "" {
		dnsUpstream = config.MihomoDNSUpstream
	}
	return ControlConfig{
		SchemaVersion: SchemaVersion, Revision: revision,
		Gateway:      GatewayConfigInput{Mode: cfg.Gateway.Mode, Interface: cfg.Gateway.Interface, LANIP: cfg.Gateway.LANIP, UpstreamInterface: cfg.Gateway.UpstreamInterface},
		DHCP:         DHCPConfigInput{Enabled: cfg.DHCP.Enabled, RangeStart: cfg.DHCP.RangeStart, RangeEnd: cfg.DHCP.RangeEnd, LeaseTime: cfg.DHCP.LeaseTime, Domain: cfg.DHCP.Domain},
		DNS:          DNSConfigInput{Listen: cfg.DNS.Listen, Upstream: dnsUpstream},
		Transparent:  TransparentConfigInput{Mode: cfg.Transparent.Mode, StrictRoute: cfg.Transparent.TUNStrictRoute},
		DevicePolicy: DevicePolicyConfigInput{Enabled: cfg.DevicePolicy.File != "", ProtectedIPv4: append([]string{}, cfg.DevicePolicy.ProtectedIPv4...)},
	}
}

func (s *Server) Serve(ctx context.Context) error {
	listener, err := net.Listen("tcp4", s.addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	descriptor := map[string]any{"schema_version": SchemaVersion, "url": s.baseURL, "pid": os.Getpid()}
	data, _ := json.MarshalIndent(descriptor, "", "  ")
	if err := writeAtomic(filepath.Join(s.store.Dir(), "control-endpoint.json"), append(data, '\n'), 0o600); err != nil {
		return err
	}
	httpServer := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	err = httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(r.Host); err == nil {
			host = parsed
		}
		if host != "127.0.0.1" && host != "localhost" {
			writeError(w, http.StatusForbidden, "invalid_host", "request host is not allowed")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		bearerOK := secureEqual(bearer, s.token)
		sessionOK := false
		if cookie, err := r.Cookie("opensurge_session"); err == nil {
			s.mu.Lock()
			expires, exists := s.sessions[cookie.Value]
			if exists && time.Now().Before(expires) {
				sessionOK = true
			} else if exists {
				delete(s.sessions, cookie.Value)
			}
			s.mu.Unlock()
		}
		if !bearerOK && !sessionOK {
			writeError(w, http.StatusUnauthorized, "authentication_required", "open the Web GUI using an authenticated launcher link")
			return
		}
		if !bearerOK && r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin != s.baseURL {
				writeError(w, http.StatusForbidden, "origin_rejected", "mutation origin is not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) exchangeBootstrap(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	s.mu.Lock()
	grant, ok := s.bootstraps[code]
	if ok {
		delete(s.bootstraps, code)
	}
	s.mu.Unlock()
	if !ok || time.Now().After(grant.expires) {
		http.Error(w, "Bootstrap link is invalid or expired", http.StatusUnauthorized)
		return
	}
	session := randomToken(32)
	s.mu.Lock()
	s.sessions[session] = time.Now().Add(12 * time.Hour)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "opensurge_session", Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 60 * 60})
	http.Redirect(w, r, "/"+grant.path, http.StatusFound)
}

func (s *Server) handleSessionBootstrap(w http.ResponseWriter, r *http.Request) {
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !secureEqual(bearer, s.token) {
		writeError(w, http.StatusUnauthorized, "authentication_required", "native launcher token is invalid")
		return
	}
	var request struct {
		Path string `json:"path"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &request, 16<<10); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	url := s.bootstrapURLFor(request.Path)
	writeJSON(w, http.StatusCreated, BootstrapResponse{SchemaVersion: SchemaVersion, URL: url, ExpiresAt: time.Now().Add(30 * time.Second).UTC()})
}

func allowedWebPath(value string) string {
	switch value {
	case "network", "recovery":
		return "network"
	case "sources", "devices", "policies", "diagnostics":
		return value
	default:
		return "dashboard"
	}
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.overview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "overview_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) overview(ctx context.Context) (Overview, error) {
	cfg, desiredErr := config.Load(s.configPath)
	if desiredErr != nil {
		cfg, _ = config.LoadRuntime(s.configPath)
	}
	manager := gateway.New(cfg)
	status, statusErr := manager.Status(ctx)
	report := doctor.Run(cfg)
	paths := runtime.NewPaths(cfg)
	leases, _ := device.LoadLeases(paths.LeaseFile)
	if leases == nil {
		leases = []device.Client{}
	}
	recovery, _ := s.store.Recovery()
	desiredDigest := ""
	if cfg.DevicePolicy.Bundle != nil {
		desiredDigest = cfg.DevicePolicy.Bundle.Digest
	}
	appliedDigest := ""
	if state, exists, _ := runtime.LoadState(paths.StateFile); exists {
		appliedDigest = state.DevicePolicyDigest
	}
	warnings := []string{}
	if desiredErr != nil {
		warnings = append(warnings, "desired configuration: "+desiredErr.Error())
	}
	groups, groupErr := mihomo.FetchProxyGroups(ctx, cfg)
	if groups == nil {
		groups = []mihomo.ProxyGroup{}
	}
	if groupErr != nil && status.Gateway == "running" {
		warnings = append(warnings, "mihomo policies unavailable: "+groupErr.Error())
	}
	providers, providerErr := mihomo.FetchProviders(ctx, cfg)
	if providers.ProxyProviders == nil {
		providers.ProxyProviders = []mihomo.ProxyProvider{}
	}
	if providers.RuleProviders == nil {
		providers.RuleProviders = []mihomo.RuleProvider{}
	}
	if providerErr != nil && status.Gateway == "running" {
		warnings = append(warnings, "mihomo providers unavailable: "+providerErr.Error())
	}
	return Overview{
		SchemaVersion: SchemaVersion,
		Revision:      fileDigest(s.configPath),
		DesiredDigest: desiredDigest,
		AppliedDigest: appliedDigest,
		Warnings:      warnings,
		Status:        status,
		StatusError:   errorString(statusErr),
		Doctor:        report.Checks,
		DoctorHealthy: doctorHealthyForControl(report.Checks),
		Leases:        leases,
		Policies:      groups,
		Providers:     providers,
		Recovery:      recovery,
	}, nil
}

func (s *Server) handleMenuBar(w http.ResponseWriter, r *http.Request) {
	overview, err := s.overview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "menubar_unavailable", err.Error())
		return
	}
	cfg, _ := config.LoadRuntime(s.configPath)
	drift := overview.DesiredDigest != "" && overview.AppliedDigest != "" && overview.DesiredDigest != overview.AppliedDigest
	writeJSON(w, http.StatusOK, MenuBarStatus{
		SchemaVersion: SchemaVersion, Revision: overview.Revision, Gateway: overview.Status.Gateway,
		Topology: cfg.Gateway.Mode, LANIP: overview.Status.LANIP, DHCP: overview.Status.DHCP,
		Mihomo: overview.Status.Mihomo, PFAnchor: overview.Status.PFAnchor, Forwarding: overview.Status.Forwarding,
		ClientCount: overview.Status.ClientCount, Drift: drift, DoctorHealthy: overview.DoctorHealthy,
		Recovery: overview.Recovery.Required, RecoveryStage: overview.Recovery.Stage, Warnings: overview.Warnings,
	})
}

func (s *Server) handleGatewayPlan(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	request := GatewayPlanRequest{}
	if r.Method == http.MethodPost && r.ContentLength != 0 {
		if err := decodeJSON(r, &request, 64<<10); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	snapshot, err := s.discoverNetwork(r.Context(), request.NetworkService, cfg.Gateway.Interface)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "network_discovery_failed", err.Error())
		return
	}
	plan := GatewayPlan{SchemaVersion: SchemaVersion, Revision: fileDigest(s.configPath), Topology: cfg.Gateway.Mode, Snapshot: snapshot, DHCPServers: []string{}, Warnings: []string{}, Blockers: []string{}}
	plan.ProtectedIPv4 = uniqueStrings(append([]string{cfg.Gateway.LANIP, snapshot.Router}, cfg.DevicePolicy.ProtectedIPv4...))
	if cfg.Gateway.Mode == config.GatewayModeSameWiFiDHCP {
		if cfg.Gateway.Interface != cfg.Gateway.UpstreamInterface {
			plan.Blockers = append(plan.Blockers, "same-LAN DHCP takeover requires one shared interface")
		}
		if snapshot.IPv4 != cfg.Gateway.LANIP {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("Mac IPv4 %s differs from configured gateway.lan_ip %s", snapshot.IPv4, cfg.Gateway.LANIP))
		}
		if snapshot.IPv6Default {
			plan.Warnings = append(plan.Warnings, "IPv6 default route is active; per-device IPv4 policy can be bypassed")
		}
		if err := s.pingRouter(r.Context(), snapshot.Router); err != nil {
			plan.Blockers = append(plan.Blockers, "upstream router is not reachable: "+err.Error())
		}
		if request.RouterDHCPDisabled {
			servers, err := s.networkRunner.ProbeDHCP(r.Context(), s.configPath, cfg.Gateway.Interface, 3*time.Second)
			if err != nil {
				plan.Blockers = append(plan.Blockers, "DHCP probe failed: "+err.Error())
			} else {
				plan.DHCPServers = servers
			}
			if len(plan.DHCPServers) > 0 {
				plan.Blockers = append(plan.Blockers, "another DHCP server is still answering: "+strings.Join(plan.DHCPServers, ", "))
			}
		}
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleGatewayAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/v1/gateway/")
	if action != "start" && action != "stop" {
		writeError(w, http.StatusNotFound, "not_found", "unknown gateway action")
		return
	}
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	if action == "start" && cfg.Gateway.Mode == config.GatewayModeSameWiFiDHCP {
		recovery, _ := s.store.Recovery()
		if recovery.Stage != RecoveryRouterDHCPDisabledConfirmed {
			writeError(w, http.StatusConflict, "recovery_precondition", "same-LAN DHCP takeover requires persisted confirmation that router DHCP is disabled")
			return
		}
	}
	id := r.Header.Get("Idempotency-Key")
	if id == "" {
		id = randomToken(12)
	}
	if existing, err := s.store.Operation(id); err == nil {
		writeJSON(w, http.StatusAccepted, existing)
		return
	}
	now := time.Now().UTC()
	op := Operation{SchemaVersion: SchemaVersion, ID: id, Kind: action, State: "running", CreatedAt: now, UpdatedAt: now}
	if err := s.store.SaveOperation(op); err != nil {
		writeError(w, http.StatusInternalServerError, "operation_failed", err.Error())
		return
	}
	go s.runOperation(op, cfg.Gateway.Mode)
	writeJSON(w, http.StatusAccepted, op)
}

func (s *Server) runOperation(op Operation, topology string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	err := s.runner.Run(ctx, op.Kind, s.configPath)
	op.UpdatedAt = time.Now().UTC()
	if err != nil {
		op.State = "failed"
		op.Error = err.Error()
	} else {
		op.State = "succeeded"
		if topology == config.GatewayModeSameWiFiDHCP {
			recovery, _ := s.store.Recovery()
			recovery.Topology = topology
			if op.Kind == "start" {
				recovery.Stage = RecoveryGatewayActive
			} else {
				recovery.Stage = RecoveryGatewayStopped
			}
			recovery.Required = true
			_ = s.store.SaveRecovery(recovery)
		}
	}
	_ = s.store.SaveOperation(op)
}

func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	op, err := s.store.Operation(r.PathValue("id"))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "operation_not_found", "operation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "operation_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (s *Server) handleOperations(w http.ResponseWriter, _ *http.Request) {
	operations, err := s.store.Operations(50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "operations_read_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "operations": operations})
}

func (s *Server) handleRecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		state, err := s.store.Recovery()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "recovery_read_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	var update RecoveryUpdate
	if err := decodeJSON(r, &update, 64<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	current, _ := s.store.Recovery()
	current.RecoveryNotes = update.RecoveryNotes
	if err := s.store.SaveRecovery(current); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) handleRecoveryCard(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.Recovery()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_read_failed", err.Error())
		return
	}
	if state.NetworkSnapshot == nil || state.Stage == RecoveryIdle {
		writeError(w, http.StatusNotFound, "recovery_card_missing", "no recovery card is available")
		return
	}
	card, err := s.store.RecoveryCard()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "recovery_card_missing", "no recovery card is available")
			return
		}
		writeError(w, http.StatusInternalServerError, "recovery_card_read_failed", err.Error())
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", disposition+`; filename="OpenSurge-WiFi-DHCP-Recovery-Card.txt"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(card)
}

func (s *Server) handleRecoveryDiscard(w http.ResponseWriter, _ *http.Request) {
	state, err := s.store.Recovery()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_read_failed", err.Error())
		return
	}
	if state.Stage != RecoveryPrepared {
		writeError(w, http.StatusConflict, "recovery_precondition", "only prepared recovery data can be discarded before network changes begin")
		return
	}
	if err := s.store.DiscardPreparedRecovery(state.Topology); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_discard_failed", err.Error())
		return
	}
	idle, _ := s.store.Recovery()
	writeJSON(w, http.StatusOK, NetworkActionResponse{SchemaVersion: SchemaVersion, Recovery: idle})
}

func (s *Server) handleClientValidated(w http.ResponseWriter, r *http.Request) {
	state, _ := s.store.Recovery()
	if state.Stage != RecoveryGatewayActive {
		writeError(w, http.StatusConflict, "recovery_precondition", "gateway must be active before client acceptance")
		return
	}
	var request ClientAcceptanceRequest
	if err := decodeJSON(r, &request, 64<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !request.GatewayDNSConfirmed || !request.NoExplicitProxyConfirmed {
		writeError(w, http.StatusUnprocessableEntity, "client_confirmation_required", "confirm client gateway/DNS and no explicit proxy")
		return
	}
	if state.NetworkSnapshot != nil && state.NetworkSnapshot.IPv6Default && !request.IPv6BypassWarningConfirmed {
		writeError(w, http.StatusUnprocessableEntity, "ipv6_warning_unacknowledged", "acknowledge that IPv6 may bypass cooperative IPv4 policy")
		return
	}
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	if err := validateClientAcceptance(cfg, request.ClientIPv4); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "client_acceptance_failed", err.Error())
		return
	}
	state.Stage = RecoveryClientValidated
	state.RecoveryNotes = fmt.Sprintf("client %s: DHCP ACK, DNS and TUN source observed; gateway/DNS and no explicit proxy confirmed", request.ClientIPv4)
	state.Required = true
	if err := s.store.SaveRecovery(state); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, NetworkActionResponse{SchemaVersion: SchemaVersion, Recovery: state})
}

func validateClientAcceptance(cfg config.Config, clientIPv4 string) error {
	if net.ParseIP(clientIPv4).To4() == nil {
		return fmt.Errorf("client_ipv4 must be valid IPv4")
	}
	paths := runtime.NewPaths(cfg)
	leases, err := device.LoadLeases(paths.LeaseFile)
	if err != nil {
		return fmt.Errorf("load leases: %w", err)
	}
	matched := false
	for _, lease := range leases {
		if lease.IP == clientIPv4 && lease.Online {
			matched = true
			break
		}
	}
	if !matched {
		return fmt.Errorf("no active DHCP lease for %s", clientIPv4)
	}
	dnsLog, _ := os.ReadFile(paths.DNSMasqLog)
	if !bytes.Contains(dnsLog, []byte("DHCPACK")) || !bytes.Contains(dnsLog, []byte(clientIPv4)) {
		return fmt.Errorf("DHCPACK evidence for %s was not found", clientIPv4)
	}
	if !bytes.Contains(dnsLog, []byte("from "+clientIPv4)) {
		return fmt.Errorf("DNS query evidence from %s was not found", clientIPv4)
	}
	mihomoLog, _ := os.ReadFile(paths.MihomoLog)
	if !bytes.Contains(mihomoLog, []byte(clientIPv4)) {
		return fmt.Errorf("mihomo TUN source evidence for %s was not found", clientIPv4)
	}
	return nil
}

func (s *Server) handleNetworkDiscovery(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	snapshot, err := s.discoverNetwork(r.Context(), r.URL.Query().Get("service"), cfg.Gateway.Interface)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "network_discovery_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleRecoveryPrepare(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	if cfg.Gateway.Mode != config.GatewayModeSameWiFiDHCP {
		writeError(w, http.StatusConflict, "same_wifi_config_required", "save the same-LAN DHCP takeover topology before preparing recovery")
		return
	}
	var request struct {
		NetworkService string `json:"network_service"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &request, 64<<10); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	snapshot, err := s.discoverNetwork(r.Context(), request.NetworkService, cfg.Gateway.Interface)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "network_discovery_failed", err.Error())
		return
	}
	if err := macosnetwork.ValidateManual(manualConfigForSnapshot(cfg, snapshot)); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "static_config_invalid", fmt.Sprintf("configured Mac LAN IPv4 %s is incompatible with router %s and subnet mask %s: %v", cfg.Gateway.LANIP, snapshot.Router, snapshot.SubnetMask, err))
		return
	}
	state := RecoveryState{SchemaVersion: SchemaVersion, Stage: RecoveryPrepared, Topology: cfg.Gateway.Mode, NetworkService: snapshot.NetworkService, OriginalIPv4: snapshot.IPv4, OriginalRouter: snapshot.Router, Required: true, NetworkSnapshot: &snapshot}
	if err := s.store.SaveRecovery(state); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	if err := s.store.SaveRecoveryCard(state); err != nil {
		_ = s.store.SaveRecovery(RecoveryState{SchemaVersion: SchemaVersion, Stage: RecoveryIdle, Topology: cfg.Gateway.Mode})
		writeError(w, http.StatusInternalServerError, "recovery_card_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, NetworkActionResponse{SchemaVersion: SchemaVersion, Recovery: state})
}

func (s *Server) handleApplyStatic(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	state, _ := s.store.Recovery()
	if state.Stage != RecoveryPrepared || state.NetworkSnapshot == nil {
		writeError(w, http.StatusConflict, "recovery_precondition", "prepare a recovery snapshot before setting static IPv4")
		return
	}
	snapshot := state.NetworkSnapshot
	manual := manualConfigForSnapshot(cfg, *snapshot)
	if err := s.networkRunner.SetManual(r.Context(), s.configPath, manual); err != nil {
		writeError(w, http.StatusBadGateway, "set_static_failed", err.Error())
		return
	}
	if err := s.pingRouter(r.Context(), snapshot.Router); err != nil {
		writeError(w, http.StatusBadGateway, "upstream_unreachable", err.Error())
		return
	}
	state.Stage, state.Required = RecoveryMacStatic, true
	if err := s.store.SaveRecovery(state); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, NetworkActionResponse{SchemaVersion: SchemaVersion, Recovery: state})
}

func manualConfigForSnapshot(cfg config.Config, snapshot macosnetwork.Snapshot) macosnetwork.ManualConfig {
	return macosnetwork.ManualConfig{NetworkService: snapshot.NetworkService, Interface: snapshot.Interface, IPv4: cfg.Gateway.LANIP, SubnetMask: snapshot.SubnetMask, Router: snapshot.Router, DNS: snapshot.DNS}
}

func (s *Server) handleDHCPProbe(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	state, _ := s.store.Recovery()
	if state.Stage != RecoveryMacStatic {
		writeError(w, http.StatusConflict, "recovery_precondition", "Mac static IPv4 must be applied before probing for router DHCP")
		return
	}
	servers, err := s.networkRunner.ProbeDHCP(r.Context(), s.configPath, cfg.Gateway.Interface, 3*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, "dhcp_probe_failed", err.Error())
		return
	}
	if len(servers) > 0 {
		writeError(w, http.StatusConflict, "competing_dhcp", "DHCP server is still answering: "+strings.Join(servers, ", "))
		return
	}
	state.Stage, state.Required = RecoveryRouterDHCPDisabledConfirmed, true
	if err := s.store.SaveRecovery(state); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, NetworkActionResponse{SchemaVersion: SchemaVersion, Recovery: state, DHCPServers: []string{}})
}

func (s *Server) handleRouterRestored(w http.ResponseWriter, r *http.Request) {
	state, _ := s.store.Recovery()
	if state.Stage != RecoveryGatewayStopped || state.NetworkSnapshot == nil {
		writeError(w, http.StatusConflict, "recovery_precondition", "stop OpenSurge before verifying restored router DHCP")
		return
	}
	servers, err := s.networkRunner.ProbeDHCP(r.Context(), s.configPath, state.NetworkSnapshot.Interface, 3*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, "dhcp_probe_failed", err.Error())
		return
	}
	if len(servers) == 0 {
		writeError(w, http.StatusConflict, "router_dhcp_missing", "no DHCP server answered after the router was marked restored")
		return
	}
	state.Stage, state.Required = RecoveryRouterDHCPRestored, true
	if err := s.store.SaveRecovery(state); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, NetworkActionResponse{SchemaVersion: SchemaVersion, Recovery: state, DHCPServers: servers})
}

func (s *Server) handleRestoreDHCP(w http.ResponseWriter, r *http.Request) {
	state, _ := s.store.Recovery()
	if state.Stage != RecoveryRouterDHCPRestored || state.NetworkSnapshot == nil {
		writeError(w, http.StatusConflict, "recovery_precondition", "verify restored router DHCP before restoring the Mac")
		return
	}
	if err := s.networkRunner.SetDHCP(r.Context(), s.configPath, state.NetworkSnapshot.NetworkService); err != nil {
		writeError(w, http.StatusBadGateway, "restore_dhcp_failed", err.Error())
		return
	}
	state.Stage, state.Required = RecoveryComplete, false
	if err := s.store.SaveRecovery(state); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, NetworkActionResponse{SchemaVersion: SchemaVersion, Recovery: state})
}

func allowedRecoveryTransition(from, to string) bool {
	if to == RecoveryPrepared && (from == RecoveryIdle || from == RecoveryComplete) {
		return true
	}
	allowed := map[string]string{
		RecoveryPrepared:                    RecoveryMacStatic,
		RecoveryMacStatic:                   RecoveryRouterDHCPDisabledConfirmed,
		RecoveryRouterDHCPDisabledConfirmed: RecoveryGatewayActive,
		RecoveryGatewayActive:               RecoveryClientValidated,
		RecoveryClientValidated:             RecoveryGatewayStopped,
		RecoveryGatewayStopped:              RecoveryRouterDHCPRestored,
		RecoveryRouterDHCPRestored:          RecoveryComplete,
	}
	return allowed[from] == to
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		sources, err := s.store.Sources()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "sources_failed", err.Error())
			return
		}
		revision := fileDigest(s.configPath)
		w.Header().Set("ETag", `"`+revision+`"`)
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "revision": revision, "sources": publicSources(sources)})
		return
	}
	var source Source
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err = r.ParseMultipartForm(maxSourceSize); err == nil {
			file, header, fileErr := r.FormFile("file")
			if fileErr != nil {
				err = fileErr
			} else {
				defer file.Close()
				name := r.FormValue("name")
				if name == "" {
					name = header.Filename
				}
				source, err = s.importReader(name, r.FormValue("kind"), "file:"+header.Filename, io.LimitReader(file, maxSourceSize+1))
			}
		}
	} else {
		var req SourceImportRequest
		if err = decodeJSON(r, &req, 64<<10); err == nil {
			source, err = s.importURL(r.Context(), req)
		}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "source_import_failed", err.Error())
		return
	}
	source.SnapshotPath = ""
	source.FetchURL = ""
	writeJSON(w, http.StatusCreated, source)
}

func (s *Server) handleSourceRefresh(w http.ResponseWriter, r *http.Request) {
	source, err := s.sourceByID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "source_not_found", err.Error())
		return
	}
	fetchURL, credentialErr := s.credentials.Get(r.Context(), source.ID)
	if credentialErr != nil || !strings.HasPrefix(fetchURL, "https://") {
		writeError(w, http.StatusConflict, "source_not_refreshable", "only HTTPS sources can be refreshed")
		return
	}
	refreshed, err := s.importURL(r.Context(), SourceImportRequest{Name: source.Name, Kind: source.Kind, URL: fetchURL})
	if err != nil {
		writeError(w, http.StatusBadRequest, "source_refresh_failed", err.Error())
		return
	}
	refreshed.SnapshotPath = ""
	refreshed.FetchURL = ""
	writeJSON(w, http.StatusOK, refreshed)
}

func (s *Server) handleSourceApply(w http.ResponseWriter, r *http.Request) {
	source, err := s.sourceByID(r.PathValue("id"))
	if err != nil || !source.Valid || source.Kind != "mihomo_profile" {
		writeError(w, http.StatusConflict, "source_not_applicable", "source must be a structurally valid mihomo profile")
		return
	}
	if _, err := config.Load(s.configPath); err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != fileDigest(s.configPath) {
		writeError(w, http.StatusConflict, "revision_conflict", "If-Match must contain the current config revision")
		return
	}
	payload, err := os.ReadFile(source.SnapshotPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "source_snapshot_unavailable", err.Error())
		return
	}
	// The privileged configuration runner performs the authoritative mihomo
	// engine validation after writing the candidate beside the persistent
	// geodata cache. Validating here as the UI user would download the same
	// assets into every source snapshot directory, then validate a second time
	// in the helper before applying.
	newRevision, err := s.configRunner.ApplyProfile(r.Context(), s.configPath, match, payload)
	if err != nil {
		status := http.StatusInternalServerError
		code := "config_apply_failed"
		if strings.Contains(err.Error(), "revision conflict") {
			status, code = http.StatusConflict, "revision_conflict"
		} else if strings.Contains(err.Error(), "mihomo config validation failed") {
			status, code = http.StatusUnprocessableEntity, "mihomo_validation_failed"
		}
		writeError(w, status, code, err.Error())
		return
	}
	sources, _ := s.store.Sources()
	for i := range sources {
		sources[i].Applied = sources[i].ID == source.ID
		for version := range sources[i].Versions {
			sources[i].Versions[version].Applied = false
		}
	}
	_ = s.store.SaveSources(sources)
	source.Applied = true
	source.SnapshotPath = ""
	source.FetchURL = ""
	w.Header().Set("ETag", `"`+newRevision+`"`)
	writeJSON(w, http.StatusOK, source)
}

func (s *Server) handleDevicePolicy(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil || cfg.DevicePolicy.File == "" {
		writeError(w, http.StatusConflict, "device_policy_unconfigured", "device_policy.file is not configured")
		return
	}
	if r.Method == http.MethodGet {
		bundle, err := device.LoadPolicyBundle(cfg.DevicePolicy.File)
		if err != nil {
			writeError(w, http.StatusBadRequest, "device_policy_invalid", err.Error())
			return
		}
		w.Header().Set("ETag", `"`+bundle.Digest+`"`)
		writeJSON(w, http.StatusOK, DevicePolicyResponse{SchemaVersion: SchemaVersion, Revision: bundle.Digest, Policy: bundle.Policy})
		return
	}
	current, err := device.LoadPolicyBundle(cfg.DevicePolicy.File)
	if err != nil {
		writeError(w, http.StatusBadRequest, "device_policy_invalid", err.Error())
		return
	}
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != current.Digest {
		writeError(w, http.StatusConflict, "revision_conflict", "If-Match must contain the current device policy revision")
		return
	}
	var policy device.PolicySet
	if err := decodeJSON(r, &policy, maxSourceSize); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := device.ValidatePolicySetForLANWithProtected(policy, cfg.Gateway.LANIP, cfg.DevicePolicy.ProtectedIPv4); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "device_policy_validation_failed", err.Error())
		return
	}
	if _, err := device.CompilePolicyBundle(policy); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "device_policy_compile_failed", err.Error())
		return
	}
	data, _ := json.Marshal(policy)
	newRevision, err := s.configRunner.ApplyDevicePolicy(r.Context(), s.configPath, match, data)
	if err != nil {
		status := http.StatusInternalServerError
		code := "device_policy_write_failed"
		if strings.Contains(err.Error(), "revision conflict") {
			status, code = http.StatusConflict, "revision_conflict"
		}
		writeError(w, status, code, err.Error())
		return
	}
	w.Header().Set("ETag", `"`+newRevision+`"`)
	writeJSON(w, http.StatusOK, DevicePolicyResponse{SchemaVersion: SchemaVersion, Revision: newRevision, Policy: policy})
}

func (s *Server) handleDevices(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil || cfg.DevicePolicy.File == "" {
		writeJSON(w, http.StatusOK, DevicesResponse{SchemaVersion: SchemaVersion, Devices: []device.CompiledDevice{}, Leases: []device.Client{}})
		return
	}
	desired, err := device.LoadPolicyBundle(cfg.DevicePolicy.File)
	if err != nil {
		writeError(w, http.StatusBadRequest, "device_policy_invalid", err.Error())
		return
	}
	paths := runtime.NewPaths(cfg)
	leases, _ := device.LoadLeases(paths.LeaseFile)
	if leases == nil {
		leases = []device.Client{}
	}
	response := DevicesResponse{SchemaVersion: SchemaVersion, DesiredDigest: desired.Digest, Devices: desired.Compiled.Devices, Leases: leases}
	if response.Devices == nil {
		response.Devices = []device.CompiledDevice{}
	}
	if state, exists, _ := runtime.LoadState(paths.StateFile); exists && state.DevicePolicyDigest != "" {
		response.Applied = true
		response.AppliedDigest = state.DevicePolicyDigest
		response.Drift = state.DevicePolicyDigest != desired.Digest
		if applied, err := device.LoadPolicyBundleSnapshot(paths.DevicePolicyApplied); err == nil {
			response.Devices = applied.Compiled.Devices
			if response.Devices == nil {
				response.Devices = []device.CompiledDevice{}
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	groups, err := mihomo.FetchProxyGroups(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "mihomo_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "groups": groups})
}

func (s *Server) handlePolicySelection(w http.ResponseWriter, r *http.Request) {
	var req SelectionRequest
	if err := decodeJSON(r, &req, 64<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	group := r.PathValue("group")
	groups, err := mihomo.FetchProxyGroups(r.Context(), cfg)
	if err != nil || !validSelection(groups, group, req.Policy) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_selection", "group or policy is not available")
		return
	}
	if err := mihomo.SelectProxyGroup(r.Context(), cfg, group, req.Policy); err != nil {
		writeError(w, http.StatusBadGateway, "selection_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "group": group, "selected": req.Policy})
}

func (s *Server) handleDeviceSelection(w http.ResponseWriter, r *http.Request) {
	var req SelectionRequest
	if err := decodeJSON(r, &req, 64<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	paths := runtime.NewPaths(cfg)
	bundle, err := device.LoadPolicyBundleSnapshot(paths.DevicePolicyApplied)
	if err != nil {
		writeError(w, http.StatusConflict, "device_policy_not_applied", err.Error())
		return
	}
	group, err := device.DeviceGroup(bundle.Policy, r.PathValue("device"), r.PathValue("slot"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_device_slot", err.Error())
		return
	}
	groups, err := mihomo.FetchProxyGroups(r.Context(), cfg)
	if err != nil || !validSelection(groups, group, req.Policy) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_selection", "device policy is not available")
		return
	}
	if err := mihomo.SelectProxyGroup(r.Context(), cfg, group, req.Policy); err != nil {
		writeError(w, http.StatusBadGateway, "selection_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "device": r.PathValue("device"), "slot": r.PathValue("slot"), "group": group, "selected": req.Policy})
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	providers, err := mihomo.FetchProviders(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "mihomo_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "providers": providers})
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	paths := runtime.NewPaths(cfg)
	connections, connectionErr := s.fetchConnections(r.Context(), cfg)
	if connections.Connections == nil {
		connections.Connections = []mihomo.Connection{}
	}
	logs := map[string][]string{
		"mihomo":  tailLines(paths.MihomoLog, 80, cfg),
		"dnsmasq": tailLines(paths.DNSMasqLog, 80, cfg),
	}
	operations, _ := s.store.Operations(20)
	recovery, _ := s.store.Recovery()
	writeJSON(w, http.StatusOK, DiagnosticsResponse{SchemaVersion: SchemaVersion, Revision: fileDigest(s.configPath), Connections: connections, ConnectionError: errorString(connectionErr), Logs: logs, Operations: operations, Recovery: recovery})
}

func tailLines(path string, limit int, cfg config.Config) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	for index := range lines {
		for _, secret := range []string{cfg.Mihomo.Secret, cfg.UpstreamProxy.Password, cfg.UpstreamProxy.Username} {
			if secret != "" {
				lines[index] = strings.ReplaceAll(lines[index], secret, "[REDACTED]")
			}
		}
	}
	return lines
}

func (s *Server) handleProviderRefresh(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	provider, err := mihomo.UpdateProxyProvider(r.Context(), cfg, r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider_refresh_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "provider": provider})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusNotImplemented, "streaming_unavailable", "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	stateTicker := time.NewTicker(2 * time.Second)
	heartbeat := time.NewTicker(15 * time.Second)
	defer stateTicker.Stop()
	defer heartbeat.Stop()
	last := ""
	sendState := func() {
		state, err := s.stateEvent(r.Context())
		if err != nil {
			return
		}
		signature, _ := json.Marshal(state)
		if string(signature) == last {
			return
		}
		last = string(signature)
		state.At = time.Now().UTC()
		data, _ := json.Marshal(state)
		fmt.Fprintf(w, "event: state\ndata: %s\n\n", data)
		flusher.Flush()
	}
	sendState()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-stateTicker.C:
			sendState()
		case now := <-heartbeat.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"at\":%q}\n\n", now.UTC().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

func (s *Server) stateEvent(ctx context.Context) (StateEvent, error) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		return StateEvent{}, err
	}
	status, _ := gateway.New(cfg).Status(ctx)
	paths := runtime.NewPaths(cfg)
	desired := ""
	if cfg.DevicePolicy.File != "" {
		if bundle, err := device.LoadPolicyBundle(cfg.DevicePolicy.File); err == nil {
			desired = bundle.Digest
		}
	}
	applied := ""
	if state, exists, _ := runtime.LoadState(paths.StateFile); exists {
		applied = state.DevicePolicyDigest
	}
	recovery, _ := s.store.Recovery()
	return StateEvent{SchemaVersion: SchemaVersion, Revision: fileDigest(s.configPath), Gateway: status.Gateway, DesiredDigest: desired, AppliedDigest: applied, Drift: applied != "" && desired != applied, Recovery: recovery}, nil
}

func (s *Server) sourceByID(id string) (Source, error) {
	sources, err := s.store.Sources()
	if err != nil {
		return Source{}, err
	}
	for _, source := range sources {
		if source.ID == id {
			return source, nil
		}
	}
	return Source{}, fmt.Errorf("source %q not found", id)
}

func publicSources(sources []Source) []Source {
	result := append([]Source{}, sources...)
	for i := range result {
		if result[i].Versions == nil {
			result[i].Versions = []SourceVersion{}
		}
		if result[i].Diff.ProxiesAdded == nil {
			result[i].Diff = diffInventory(result[i].Diff.PreviousDigest, Inventory{}, Inventory{})
		}
		result[i].SnapshotPath = ""
		result[i].FetchURL = ""
		for version := range result[i].Versions {
			result[i].Versions[version].SnapshotPath = ""
		}
	}
	return result
}

func validSelection(groups []mihomo.ProxyGroup, group, policy string) bool {
	for _, candidate := range groups {
		if candidate.Name != group {
			continue
		}
		for _, option := range candidate.Options {
			if option == policy {
				return true
			}
		}
	}
	return false
}

func doctorHealthyForControl(checks []doctor.Check) bool {
	for _, check := range checks {
		if check.Name == "root privileges" {
			continue
		}
		if !check.OK {
			return false
		}
	}
	return true
}

func fileDigest(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func decodeJSON(r *http.Request, value any, limit int64) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{SchemaVersion: SchemaVersion, Error: APIError{Code: code, Message: message}})
}

func randomToken(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data)
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
