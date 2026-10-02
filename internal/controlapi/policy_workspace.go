package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

// PolicyWorkspaceRequest never accepts engine configuration or credentials from
// the browser. Source/overlay inputs below are populated by the control service.
type PolicyWorkspaceRequest struct {
	Action string   `json:"action"`
	Group  string   `json:"group,omitempty"`
	Policy string   `json:"policy,omitempty"`
	Names  []string `json:"names,omitempty"`
}

type PolicyWorkspaceInput struct {
	Request              PolicyWorkspaceRequest `json:"request"`
	Revision             string                 `json:"revision"`
	ExpectedGatewayState string                 `json:"expected_gateway_state"`
	Source               []byte                 `json:"source,omitempty"`
	Overlay              []byte                 `json:"overlay,omitempty"`
}

const (
	policyWorkspaceGatewayRunning = "running"
	policyWorkspaceGatewayStopped = "stopped"
)

type PolicyWorkspaceResponse struct {
	SchemaVersion int                        `json:"schema_version"`
	Mode          string                     `json:"mode"`
	Revision      string                     `json:"revision"`
	Groups        []mihomo.ProxyGroup        `json:"groups"`
	Health        mihomo.ProxyHealthSnapshot `json:"health"`
	Results       []mihomo.ProxyDelayResult  `json:"results,omitempty"`
}

type PolicyWorkspaceRunner interface {
	PolicyWorkspace(context.Context, string, PolicyWorkspaceInput) (PolicyWorkspaceResponse, error)
	HoldPolicyWorkspace(context.Context, string) (io.Closer, error)
}

type policyWorkspaceLease struct {
	mu     sync.Mutex
	closer io.Closer
	closed bool
}

func (l *policyWorkspaceLease) hold(ctx context.Context, runner PolicyWorkspaceRunner, path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return fmt.Errorf("control service is shutting down")
	}
	if l.closer != nil {
		return nil
	}
	closer, err := runner.HoldPolicyWorkspace(ctx, path)
	if err == nil {
		l.closer = closer
	}
	return err
}

func (l *policyWorkspaceLease) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.closer == nil {
		return nil
	}
	err := l.closer.Close()
	l.closer = nil
	return err
}

// reset drops a failed Helper connection without permanently closing the
// workspace. A restarted Helper cannot reuse the old lease socket, so the
// next browser request must be allowed to establish a fresh lease.
func (l *policyWorkspaceLease) reset() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closer == nil {
		return nil
	}
	err := l.closer.Close()
	l.closer = nil
	return err
}

func (s *Server) handlePolicyWorkspace(w http.ResponseWriter, r *http.Request) {
	var request PolicyWorkspaceRequest
	if err := decodeJSON(r, &request, 128<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validatePolicyWorkspaceRequest(request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_policy_request", err.Error())
		return
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	endActivity := s.gatewayActivity.beginPolicyWorkspace()
	defer endActivity()
	input, err := s.policyWorkspaceInput(request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "policy_workspace_invalid", err.Error())
		return
	}
	if err := s.policyWorkspaceLease.hold(r.Context(), s.policyWorkspaceRunner, s.configPath); err != nil {
		writeError(w, http.StatusBadGateway, "policy_workspace_unavailable", err.Error())
		return
	}
	if request.Action == "test" && strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		s.streamPolicyWorkspace(w, r, input)
		return
	}
	response, err := s.policyWorkspaceRunner.PolicyWorkspace(r.Context(), s.configPath, input)
	if err != nil {
		_ = s.policyWorkspaceLease.reset()
		writeError(w, http.StatusBadGateway, "policy_workspace_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func validatePolicyWorkspaceRequest(request PolicyWorkspaceRequest) error {
	switch request.Action {
	case "read":
	case "select":
		if request.Group == "" || request.Policy == "" || mihomo.IsLocalRoutingGroup(request.Group) || mihomo.IsLocalRoutingGroup(request.Policy) {
			return fmt.Errorf("choose an available user policy group and member")
		}
	case "test":
		if len(uniqueProxyNames(request.Names)) == 0 || len(request.Names) > 128 {
			return fmt.Errorf("choose between 1 and 128 nodes")
		}
	default:
		return fmt.Errorf("unsupported policy workspace action")
	}
	return nil
}

func (s *Server) policyWorkspaceInput(request PolicyWorkspaceRequest) (PolicyWorkspaceInput, error) {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		return PolicyWorkspaceInput{}, err
	}
	input := PolicyWorkspaceInput{Request: request, Revision: fileDigest(s.configPath)}
	_, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile)
	if exists {
		input.ExpectedGatewayState = policyWorkspaceGatewayRunning
	} else {
		input.ExpectedGatewayState = policyWorkspaceGatewayStopped
	}
	if err != nil || exists {
		return input, err
	} // Running views never consume drafts.
	var overlay mihomo.ProfileOverlayDocument
	var overlayRevision string
	input.Overlay, overlay, overlayRevision, err = s.loadProfileOverlay()
	if err != nil {
		return input, err
	}
	if cfg.Mihomo.ProfileMode != config.MihomoProfileModeImported {
		return input, nil
	}
	effectiveOverlayRevision := ""
	if overlay.Enabled {
		effectiveOverlayRevision = overlayRevision
	}
	// The persisted imported profile already represents this composition. Its
	// root-owned copy is authoritative and remains usable when an optional user
	// source-library snapshot has been deleted or is temporarily unavailable.
	if cfg.Mihomo.ProfileOverlayDigest == effectiveOverlayRevision {
		return input, nil
	}
	digest := cfg.Mihomo.ProfileSourceDigest
	if digest == "" {
		digest, _ = config.MihomoProfileDigest(cfg)
	}
	sources, err := s.store.Sources()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return input, err
	}
	for _, source := range sources {
		path := ""
		if source.Digest == digest {
			path = source.SnapshotPath
		}
		for _, version := range source.Versions {
			if version.Digest == digest {
				path = version.SnapshotPath
				break
			}
		}
		if path == "" {
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil || mihomo.ProfileOverlayDigest(data) != digest {
			// Source-library snapshots are optional inputs, never authority over a
			// valid root-owned desired profile. workspaceCandidate will use its
			// immutable base metadata or current profile, and will fail honestly if
			// neither can support the requested recomposition.
			continue
		}
		input.Source = data
		break
	}
	return input, nil
}

// Workspace base metadata makes repeated composition idempotent, including the
// overlay-only case. It never replaces a user's original imported source file.
type policyWorkspaceBase struct {
	EffectivePath string                     `json:"effective_path"`
	ProfileMode   string                     `json:"profile_mode"`
	ProfilePath   string                     `json:"profile_path"`
	Source        []byte                     `json:"source"`
	SourceDigest  string                     `json:"source_digest"`
	UpstreamProxy config.UpstreamProxyConfig `json:"upstream_proxy"`
}

func policyWorkspaceBasePath(cfg config.Config) string {
	// This metadata must not live below mihomo's -d directory. Provider cache
	// paths are writable by the engine and a perfectly valid relative path must
	// never be able to replace OpenSurge's composition recovery record.
	return filepath.Join(cfg.Runtime.Dir, "policy-workspace", "base.json")
}

func policyWorkspaceProfilePath(configPath, digest string) string {
	// Keep the profile in the managed data root: imported type:file providers
	// and the managed Tailscale state rely on that directory remaining mihomo's
	// SAFE_PATHS root. Provider paths and caches retain their existing semantics.
	return filepath.Join(filepath.Dir(configPath), "data", "workspace-profile-"+digest[:16]+".yaml")
}

func workspaceCandidate(configPath string, cfg config.Config, input PolicyWorkspaceInput) (config.Config, *policyWorkspaceBase, error) {
	if err := config.PrepareDevicePolicy(&cfg); err != nil {
		return cfg, nil, err
	}
	document := mihomo.DefaultProfileOverlayDocument()
	if len(strings.TrimSpace(string(input.Overlay))) > 0 {
		var err error
		document, err = mihomo.ParseProfileOverlay(input.Overlay)
		if err != nil {
			return cfg, nil, err
		}
	}
	if !document.Enabled && cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported && cfg.Mihomo.ProfileOverlayDigest == "" {
		if len(input.Source) > 0 && cfg.Mihomo.ProfileSourceDigest != "" && cfg.Mihomo.ProfileSourceDigest != mihomo.ProfileOverlayDigest(input.Source) {
			return cfg, nil, fmt.Errorf("selected profile changed")
		}
		if _, err := os.Stat(cfg.Mihomo.Profile); err != nil {
			return cfg, nil, err
		}
		return cfg, nil, nil
	}
	base := policyWorkspaceBase{ProfileMode: cfg.Mihomo.ProfileMode, ProfilePath: cfg.Mihomo.Profile, UpstreamProxy: cfg.UpstreamProxy}
	metadataPath := policyWorkspaceBasePath(cfg)
	if data, err := os.ReadFile(metadataPath); err == nil {
		var previous policyWorkspaceBase
		if err := json.Unmarshal(data, &previous); err != nil {
			return cfg, nil, fmt.Errorf("read policy workspace base: %w", err)
		}
		if previous.EffectivePath == cfg.Mihomo.Profile {
			base = previous
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, nil, err
	}
	if len(input.Source) > 0 {
		digest := mihomo.ProfileOverlayDigest(input.Source)
		if cfg.Mihomo.ProfileSourceDigest != "" && cfg.Mihomo.ProfileSourceDigest != digest {
			return cfg, nil, fmt.Errorf("selected profile changed")
		}
		base.Source = input.Source
		base.SourceDigest = digest
	} else if base.ProfileMode == config.MihomoProfileModeManaged {
		text, err := mihomo.RenderManagedBaseProfile(cfg)
		if err != nil {
			return cfg, nil, err
		}
		base.Source = []byte(text)
		base.SourceDigest = mihomo.ProfileOverlayDigest(base.Source)
	} else if len(base.Source) == 0 {
		if cfg.Mihomo.ProfileOverlayDigest != "" {
			// Legacy applied profiles can already include an overlay. Without their
			// raw source, reapplying a different overlay would duplicate additions.
			if cfg.Mihomo.ProfileOverlayDigest == mihomo.ProfileOverlayDigest(input.Overlay) {
				return cfg, nil, nil
			}
			return cfg, nil, fmt.Errorf("raw source is unavailable; import or select the source before changing its extension")
		}
		data, err := os.ReadFile(base.ProfilePath)
		if err != nil {
			return cfg, nil, err
		}
		base.Source = data
		base.SourceDigest = mihomo.ProfileOverlayDigest(data)
	}
	if !document.Enabled && base.ProfileMode == config.MihomoProfileModeManaged {
		cfg.Mihomo.ProfileMode, cfg.Mihomo.Profile = base.ProfileMode, base.ProfilePath
		cfg.Mihomo.ProfileSourceDigest, cfg.Mihomo.ProfileOverlayDigest = "", ""
		cfg.UpstreamProxy = base.UpstreamProxy
		return cfg, nil, nil
	}
	composition, err := mihomo.ComposeProfileOverlay(base.Source, document)
	if err != nil {
		return cfg, nil, err
	}
	payload := []byte(composition.ProfileYAML)
	digest := mihomo.ProfileOverlayDigest(payload)
	base.EffectivePath = policyWorkspaceProfilePath(configPath, digest)
	if err := writeWorkspaceArtifact(base.EffectivePath, payload, 0o640); err != nil {
		return cfg, nil, err
	}
	cfg.Mihomo.ProfileMode, cfg.Mihomo.Profile = config.MihomoProfileModeImported, base.EffectivePath
	cfg.Mihomo.ProfileSourceDigest = base.SourceDigest
	cfg.Mihomo.ProfileOverlayDigest = ""
	// The managed upstream is already embedded in the materialized profile.
	// Imported mode deliberately disables the parallel smoke-proxy renderer.
	if base.ProfileMode == config.MihomoProfileModeManaged {
		cfg.UpstreamProxy.Enabled = false
	}
	if document.Enabled {
		cfg.Mihomo.ProfileOverlayDigest = mihomo.ProfileOverlayDigest(input.Overlay)
	}
	return cfg, &base, nil
}

func (DirectRunner) PolicyWorkspace(ctx context.Context, configPath string, input PolicyWorkspaceInput) (response PolicyWorkspaceResponse, err error) {
	if os.Geteuid() != 0 {
		return response, fmt.Errorf("privileged helper is required")
	}
	return runPolicyWorkspace(ctx, configPath, input)
}

type policyWorkspaceStartDeps struct {
	geteuid     func() int
	startLocked func(context.Context, config.Config, func() error) error
}

// startPolicyWorkspace turns the server-authoritative source and overlay
// snapshot into the desired gateway configuration and starts that exact
// candidate under one lifecycle lock. A user therefore never has to visit the
// Policies page merely to materialize a source-free global overlay, and a
// concurrent CLI invocation cannot start the previous graph in the gap.
func startPolicyWorkspace(ctx context.Context, configPath string, input PolicyWorkspaceInput, deps policyWorkspaceStartDeps) error {
	ctx, cancel := context.WithTimeout(ctx, policyWorkspaceStartTimeout)
	defer cancel()
	if deps.geteuid == nil || deps.geteuid() != 0 {
		return fmt.Errorf("privileged helper is required")
	}
	if input.Request.Action != "read" || input.Request.Group != "" || input.Request.Policy != "" || len(input.Request.Names) != 0 {
		return fmt.Errorf("gateway start requires a read-only policy workspace snapshot")
	}
	if input.ExpectedGatewayState != policyWorkspaceGatewayStopped {
		return fmt.Errorf("gateway start requires a snapshot captured while the gateway was stopped")
	}
	if deps.startLocked == nil {
		return fmt.Errorf("gateway start transaction is unavailable")
	}
	cfg, err := config.LoadRuntime(configPath)
	if err != nil {
		return err
	}
	return runtime.WithLifecycleLock(cfg, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if input.Revision != fileDigest(configPath) {
			return fmt.Errorf("config revision conflict; retry gateway start")
		}
		if _, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile); err != nil {
			return err
		} else if err := requirePolicyWorkspaceSnapshotState(input, exists); err != nil {
			return err
		}
		candidate, base, err := workspaceCandidate(configPath, cfg, input)
		if err != nil {
			return err
		}
		// The gateway manager resolves network parameters and validates the final
		// rendered file once. It invokes this commit before any network takeover.
		return deps.startLocked(ctx, candidate, func() error {
			return persistPolicyWorkspaceCandidate(ctx, configPath, cfg, candidate, base)
		})
	})
}

func runPolicyWorkspace(ctx context.Context, configPath string, input PolicyWorkspaceInput) (response PolicyWorkspaceResponse, err error) {
	if err := validatePolicyWorkspaceRequest(input.Request); err != nil {
		return response, err
	}
	cfg, err := config.LoadRuntime(configPath)
	if err != nil {
		return response, err
	}
	err = runtime.WithLifecycleLock(cfg, func() error {
		if input.Revision != fileDigest(configPath) {
			return fmt.Errorf("config revision conflict; refresh policies")
		}
		state, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile)
		if err != nil {
			return err
		}
		if err := requirePolicyWorkspaceSnapshotState(input, exists); err != nil {
			return err
		}
		response.Mode = "running"
		apiConfig := cfg
		if exists {
			boot, err := runtime.CurrentBootSession()
			if err != nil {
				return err
			}
			if !state.BelongsToBoot(boot) {
				return fmt.Errorf("gateway runtime was interrupted; stop it before preparing policies")
			}
		} else {
			response.Mode = "prepared"
			candidate, _, err := workspaceCandidate(configPath, cfg, input)
			if err != nil {
				return err
			}
			if err := mihomo.PrepareDevicePolicy(&candidate); err != nil {
				return err
			}
			final, err := mihomo.RenderConfig(candidate)
			if err != nil {
				return err
			}
			prepared, err := mihomo.PrepareLocked(ctx, candidate, final)
			if err != nil {
				return err
			}
			apiConfig = mihomo.PreparedConfig(candidate, prepared)
		}
		groups, err := mihomo.FetchProxyGroups(ctx, apiConfig)
		if err != nil {
			return err
		}
		groups = mihomo.VisibleProxyGroups(groups)
		if input.Request.Action == "select" {
			if !validSelection(groups, input.Request.Group, input.Request.Policy) {
				return fmt.Errorf("group or member is unavailable")
			}
			if err := mihomo.SelectProxyGroup(ctx, apiConfig, input.Request.Group, input.Request.Policy); err != nil {
				return err
			}
			groups, err = mihomo.FetchProxyGroups(ctx, apiConfig)
			if err != nil {
				return err
			}
			groups = mihomo.VisibleProxyGroups(groups)
		}
		health, err := mihomo.FetchProxyHealth(ctx, apiConfig)
		if err != nil {
			return err
		}
		health.Proxies = mihomo.VisibleProxyHealth(health.Proxies)
		if input.Request.Action == "test" {
			available := make(map[string]mihomo.ProxyHealth)
			for _, proxy := range health.Proxies {
				available[proxy.Name] = proxy
			}
			names := uniqueProxyNames(input.Request.Names)
			for _, name := range names {
				proxy, ok := available[name]
				if !ok || !proxy.Probeable {
					return fmt.Errorf("node is unavailable or cannot be tested")
				}
			}
			response.Results = measurePolicyWorkspaceNodes(ctx, names, func(ctx context.Context, name string) mihomo.ProxyDelayResult {
				url, timeout := proxyHealthProbe(available[name], health.TestURL)
				return mihomo.MeasureProxyDelay(ctx, apiConfig, name, url, timeout)
			})
			if err := ctx.Err(); err != nil {
				return err
			}
			health, err = mihomo.FetchProxyHealth(ctx, apiConfig)
			if err != nil {
				return err
			}
			health.Proxies = mihomo.VisibleProxyHealth(health.Proxies)
		}
		response.SchemaVersion, response.Revision = SchemaVersion, fileDigest(configPath)
		response.Groups, response.Health = groups, health
		if response.Groups == nil {
			response.Groups = []mihomo.ProxyGroup{}
		}
		if response.Health.Proxies == nil {
			response.Health.Proxies = []mihomo.ProxyHealth{}
		}
		return nil
	})
	return response, err
}

func requirePolicyWorkspaceSnapshotState(input PolicyWorkspaceInput, exists bool) error {
	actual := policyWorkspaceGatewayStopped
	if exists {
		actual = policyWorkspaceGatewayRunning
	}
	if input.ExpectedGatewayState != policyWorkspaceGatewayRunning && input.ExpectedGatewayState != policyWorkspaceGatewayStopped {
		return fmt.Errorf("policy workspace snapshot has no valid gateway state; refresh policies")
	}
	if input.ExpectedGatewayState != actual {
		return fmt.Errorf("gateway state changed after the policy snapshot; refresh policies and retry")
	}
	return nil
}

// persistPolicyWorkspaceCandidate commits an explicitly applied candidate.
// Merely reading, selecting or testing the prepared graph never calls it.
func persistPolicyWorkspaceCandidate(ctx context.Context, configPath string, previous, candidate config.Config, base *policyWorkspaceBase) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	basePath := policyWorkspaceBasePath(candidate)
	oldBase, err := snapshotFile(basePath)
	if err != nil {
		return err
	}
	oldConfig, err := snapshotFile(configPath)
	if err != nil {
		return err
	}
	baseWritten, configWritten := false, false
	restore := func(cause error) error {
		if configWritten {
			cause = errors.Join(cause, restoreFile(configPath, oldConfig))
		}
		if baseWritten {
			cause = errors.Join(cause, restoreFile(basePath, oldBase))
		}
		return cause
	}
	if base != nil {
		data, err := json.Marshal(base)
		if err != nil {
			return err
		}
		if err := writeWorkspaceArtifact(basePath, data, 0o600); err != nil {
			return err
		}
		baseWritten = true
	}
	if err := ctx.Err(); err != nil {
		return restore(err)
	}
	if rendered := config.Render(candidate); rendered != config.Render(previous) {
		if err := writeAtomic(configPath, []byte(rendered), 0o640); err != nil {
			return restore(err)
		}
		configWritten = true
	}
	if err := ctx.Err(); err != nil {
		return restore(err)
	}
	return nil
}

func writeWorkspaceArtifact(path string, data []byte, mode os.FileMode) error {
	previous, err := os.ReadFile(path)
	if err == nil && string(previous) == string(data) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAtomic(path, data, mode)
}
