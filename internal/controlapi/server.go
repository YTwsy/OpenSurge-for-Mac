package controlapi

import (
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
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

type Options struct {
	ConfigPath string
	Addr       string
	StoreDir   string
	Runner     ActionRunner
	Static     http.Handler
}

type Server struct {
	configPath string
	addr       string
	store      *Store
	runner     ActionRunner
	static     http.Handler
	token      string
	baseURL    string

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
	return &Server{
		configPath: configPath,
		addr:       options.Addr,
		store:      store,
		runner:     options.Runner,
		static:     options.Static,
		token:      token,
		baseURL:    "http://" + options.Addr,
		sessions:   map[string]time.Time{},
		bootstraps: map[string]bootstrapGrant{},
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
	mux.Handle("GET /api/v1/menubar", s.auth(http.HandlerFunc(s.handleMenuBar)))
	mux.Handle("GET /api/v1/gateway/plan", s.auth(http.HandlerFunc(s.handleGatewayPlan)))
	mux.Handle("POST /api/v1/gateway/start", s.auth(http.HandlerFunc(s.handleGatewayAction)))
	mux.Handle("POST /api/v1/gateway/stop", s.auth(http.HandlerFunc(s.handleGatewayAction)))
	mux.Handle("GET /api/v1/recovery", s.auth(http.HandlerFunc(s.handleRecovery)))
	mux.Handle("POST /api/v1/recovery", s.auth(http.HandlerFunc(s.handleRecovery)))
	mux.Handle("GET /api/v1/sources", s.auth(http.HandlerFunc(s.handleSources)))
	mux.Handle("POST /api/v1/sources", s.auth(http.HandlerFunc(s.handleSources)))
	mux.Handle("POST /api/v1/sources/{id}/refresh", s.auth(http.HandlerFunc(s.handleSourceRefresh)))
	mux.Handle("POST /api/v1/sources/{id}/apply", s.auth(http.HandlerFunc(s.handleSourceApply)))
	mux.Handle("GET /api/v1/device-policy", s.auth(http.HandlerFunc(s.handleDevicePolicy)))
	mux.Handle("PUT /api/v1/device-policy", s.auth(http.HandlerFunc(s.handleDevicePolicy)))
	mux.Handle("GET /api/v1/devices", s.auth(http.HandlerFunc(s.handleDevices)))
	mux.Handle("POST /api/v1/devices/{device}/selectors/{slot}", s.auth(http.HandlerFunc(s.handleDeviceSelection)))
	mux.Handle("GET /api/v1/policies", s.auth(http.HandlerFunc(s.handlePolicies)))
	mux.Handle("POST /api/v1/policies/{group}/selection", s.auth(http.HandlerFunc(s.handlePolicySelection)))
	mux.Handle("GET /api/v1/providers", s.auth(http.HandlerFunc(s.handleProviders)))
	mux.Handle("POST /api/v1/providers/{name}/refresh", s.auth(http.HandlerFunc(s.handleProviderRefresh)))
	mux.Handle("GET /api/v1/operations/{id}", s.auth(http.HandlerFunc(s.handleOperation)))
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
	overview, err := s.overview(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, "plan_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
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
			writeError(w, http.StatusConflict, "recovery_precondition", "same-WiFi start requires persisted confirmation that router DHCP is disabled")
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
	if !allowedRecoveryTransition(current.Stage, update.Stage) {
		writeError(w, http.StatusConflict, "invalid_recovery_transition", fmt.Sprintf("cannot move recovery from %s to %s", current.Stage, update.Stage))
		return
	}
	cfg, _ := config.LoadRuntime(s.configPath)
	current.Stage = update.Stage
	current.Topology = cfg.Gateway.Mode
	current.NetworkService = update.NetworkService
	current.OriginalIPv4 = update.OriginalIPv4
	current.OriginalRouter = update.OriginalRouter
	current.RecoveryNotes = update.RecoveryNotes
	current.Required = update.Stage != RecoveryIdle && update.Stage != RecoveryComplete
	if err := s.store.SaveRecovery(current); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func allowedRecoveryTransition(from, to string) bool {
	if to == RecoveryPrepared && (from == RecoveryIdle || from == RecoveryComplete) {
		return true
	}
	allowed := map[string]string{
		RecoveryPrepared:                    RecoveryMacStatic,
		RecoveryMacStatic:                   RecoveryRouterDHCPDisabledConfirmed,
		RecoveryRouterDHCPDisabledConfirmed: RecoveryGatewayActive,
		RecoveryGatewayActive:               RecoveryGatewayStopped,
		RecoveryGatewayStopped:              RecoveryRouterDHCPRestored,
		RecoveryRouterDHCPRestored:          RecoveryComplete,
	}
	return allowed[from] == to
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		sources, err := s.store.Sources()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "sources_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "sources": publicSources(sources)})
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
	if !strings.HasPrefix(source.FetchURL, "https://") {
		writeError(w, http.StatusConflict, "source_not_refreshable", "only HTTPS sources can be refreshed")
		return
	}
	refreshed, err := s.importURL(r.Context(), SourceImportRequest{Name: source.Name, Kind: source.Kind, URL: source.FetchURL})
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
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config_invalid", err.Error())
		return
	}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = source.SnapshotPath
	temp, err := os.MkdirTemp("", "opensurge-source-validate-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "validation_failed", err.Error())
		return
	}
	defer os.RemoveAll(temp)
	validation := cfg
	validation.Runtime.Dir = temp
	validation.Mihomo.Config = filepath.Join(temp, "mihomo.yaml")
	if err := mihomo.New(validation, runtime.NewPaths(validation)).ValidateConfig(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "mihomo_validation_failed", err.Error())
		return
	}
	if err := writeAtomic(s.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, "config_write_failed", err.Error())
		return
	}
	sources, _ := s.store.Sources()
	for i := range sources {
		sources[i].Applied = sources[i].ID == source.ID
	}
	_ = s.store.SaveSources(sources)
	source.Applied = true
	source.SnapshotPath = ""
	source.FetchURL = ""
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
	bundle, err := device.CompilePolicyBundle(policy)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "device_policy_compile_failed", err.Error())
		return
	}
	data, _ := json.MarshalIndent(policy, "", "  ")
	if err := writeAtomic(cfg.DevicePolicy.File, append(data, '\n'), 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, "device_policy_write_failed", err.Error())
		return
	}
	w.Header().Set("ETag", `"`+bundle.Digest+`"`)
	writeJSON(w, http.StatusOK, DevicePolicyResponse{SchemaVersion: SchemaVersion, Revision: bundle.Digest, Policy: policy})
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
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case now := <-ticker.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"at\":%q}\n\n", now.UTC().Format(time.RFC3339))
			flusher.Flush()
		}
	}
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
		result[i].SnapshotPath = ""
		result[i].FetchURL = ""
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
